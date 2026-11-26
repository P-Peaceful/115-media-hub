// client.go：执行有期限、有大小上限的 115 协议请求，统一凭据校验、只读重试、限流冷却与端点信任。

package provider115

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 持有独立 HTTP 客户端与账号调度器；构造后跨服务和分页复用，Close 释放等待者。
type Client struct {
	clientState
	http        *http.Client
	interval    time.Duration
	testOrigins map[string]bool
	logger      *slog.Logger
}

// New 构造禁用重定向的传输；间隔必须为 1–60 秒，零值采用 1 秒。
func New(config Config) (*Client, error) {
	interval := config.Interval
	if interval == 0 {
		interval = time.Second
	}
	if interval < time.Second || interval > time.Minute {
		return nil, &Error{Kind: Protocol}
	}
	c := &Client{clientState: clientState{accounts: make(map[string]*accountGate), done: make(chan struct{})}, http: &http.Client{Transport: config.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, interval: interval, testOrigins: make(map[string]bool), logger: config.Logger}
	for _, endpoint := range config.TestEndpoints {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, &Error{Kind: Protocol}
		}
		c.testOrigins[u.Scheme+"://"+u.Host] = true
	}
	return c, nil
}

// Close 停止新请求并唤醒等待者；在途请求仍由调用方 context 控制。
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
}

// validEndpoint 只允许声明操作对应的固定生产路径，测试显式注入的 origin 不放宽生产域名。
func (c *Client) validEndpoint(r Request) bool {
	u, err := url.Parse(r.URL)
	if err != nil || u.User != nil || u.Fragment != "" || u.Host == "" || u.Opaque != "" {
		return false
	}
	if c.testOrigins[u.Scheme+"://"+u.Host] {
		return true
	}
	if u.Scheme != "https" {
		return false
	}
	switch r.Operation {
	case QR, Login:
		return u.Host == "qrcodeapi.115.com" && (u.Path == "/api/1.0/web/1.0/token/" || u.Path == "/get/status/" || u.Path == "/api/1.0/mac/1.0/qrcode" || u.Path == "/app/1.0/alipaymini/1.0/login/qrcode/")
	case Metadata:
		return (u.Host == "my.115.com" && u.Path == "/") || (u.Host == "webapi.115.com" && (u.Path == "/files/index_info" || u.Path == "/category/get"))
	case Files, FastFiles:
		return u.Host == "webapi.115.com" && u.Path == "/files"
	case Playback, DownloadLink:
		return u.Host == "proapi.115.com" && u.Path == "/app/chrome/downurl"
	case Directories:
		return u.Host == "proapi.115.com" && u.Path == "/app/chrome/downfolders"
	}
	return false
}

// policy 返回显式操作的只读性、总期限及最大响应；未知操作绝不发出请求。
func policy(op Operation) (time.Duration, int64, bool, bool) {
	switch op {
	case QR, Login:
		return 45 * time.Second, 1 << 20, false, true
	case Metadata, Files:
		return 45 * time.Second, 1 << 20, true, true
	case FastFiles:
		return 45 * time.Second, 4 << 20, true, true
	case Directories:
		return 45 * time.Second, 8 << 20, true, true
	case Playback:
		return 15 * time.Second, 64 << 10, true, true
	case DownloadLink:
		return 45 * time.Second, 64 << 10, true, true
	default:
		return 0, 0, false, false
	}
}

// Do 在同一总期限内重建并执行有限次请求；预算、凭据和取消在实际发送边界再次校验。
func (c *Client) Do(parent context.Context, r Request) ([]byte, error) {
	total, maxBytes, retry, known := policy(r.Operation)
	if !known || !c.validEndpoint(r) || (r.Operation != QR && r.Operation != Login && r.Scope == "") || (r.Priority != Foreground && r.Priority != Background) {
		return nil, &Error{Operation: r.Operation, Kind: Protocol}
	}
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	allowedMethod := r.Method == http.MethodGet || r.Method == http.MethodPost && (r.Operation == Login || r.Operation == Playback || r.Operation == DownloadLink)
	if !allowedMethod {
		return nil, &Error{Operation: r.Operation, Kind: Protocol}
	}
	if r.MaxBytes > 0 {
		if r.MaxBytes > maxBytes {
			return nil, &Error{Operation: r.Operation, Kind: Protocol}
		}
		maxBytes = r.MaxBytes
	}
	if r.Priority == Background && retry && r.Operation != Playback {
		// 后台页至少容纳配置间隔加一次在途请求，避免合法的长间隔使后续页永远无法准入。
		total = max(total, c.interval+15*time.Second)
	}
	ctx, cancel := context.WithTimeout(parent, total)
	defer cancel()
	budget := r.Budget
	if budget == nil {
		budget, _ = ctx.Value(budgetKey{}).(*Budget)
	}
	attempts := 1
	if retry {
		attempts = 3
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, contextError(r.Operation, err)
		}
		if budget != nil && budget.Remaining() == 0 {
			return nil, ErrBudgetExhausted
		}
		var gate *accountGate
		if r.Operation != QR && r.Operation != Login {
			var err error
			gate, err = c.acquire(ctx, r.Scope, r.Priority)
			if err != nil {
				if ctx.Err() != nil {
					return nil, contextError(r.Operation, ctx.Err())
				}
				return nil, err
			}
		} else {
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return nil, ErrClosed
			}
		}
		data, err := c.attempt(ctx, r, maxBytes, budget, gate)
		if gate != nil {
			c.release(gate)
		}
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, contextError(r.Operation, ctx.Err())
		}
		var upstream *Error
		if !retry || attempt == attempts || !errors.As(err, &upstream) || (upstream.Kind != Temporary && upstream.Kind != Throttled) {
			return nil, err
		}
		if budget != nil && budget.Remaining() == 0 {
			return nil, ErrBudgetExhausted
		}
		delay := time.Duration(attempt) * time.Second
		if upstream.Kind == Throttled {
			c.mu.Lock()
			blocked := gate.blocked
			until := gate.cooldown
			c.mu.Unlock()
			deadline, _ := ctx.Deadline()
			if blocked || !until.Before(deadline) {
				return nil, err
			}
			delay = time.Until(until)
		}
		if c.logger != nil {
			c.logger.DebugContext(ctx, "115 read retry", slog.String("component", "adapter"), slog.String("operation", string(r.Operation)), slog.Int("attempt_count", attempt), slog.String("error_code", string(upstream.Kind)), slog.Int64("wait_ms", delay.Milliseconds()))
		}
		if waitErr := c.waitContext(ctx, delay); waitErr != nil {
			return nil, contextError(r.Operation, waitErr)
		}
	}
	return nil, &Error{Operation: r.Operation, Kind: Protocol}
}

// attempt 持有账号在途资格直至响应关闭；任何网络文本都不进入返回错误。
func (c *Client) attempt(ctx context.Context, r Request, maxBytes int64, budget *Budget, gate *accountGate) ([]byte, error) {
	if r.Check != nil {
		if err := r.Check(ctx); err != nil {
			return nil, checkError(r.Operation, err)
		}
	}
	attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, &Error{Operation: r.Operation, Kind: Protocol}
	}
	req.Header = r.Header.Clone()
	if err := ctx.Err(); err != nil {
		return nil, contextError(r.Operation, err)
	}
	if budget != nil && !budget.Take() {
		return nil, ErrBudgetExhausted
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, contextError(r.Operation, ctx.Err())
		}
		return nil, &Error{Operation: r.Operation, Kind: Temporary}
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	closeErr := resp.Body.Close()
	var throttle *Error
	if resp.StatusCode == http.StatusTooManyRequests {
		until, blocked := retryAfter(resp.Header.Get("Retry-After"), time.Now())
		if gate != nil {
			c.cool(gate, until, blocked)
		}
		if c.logger != nil {
			c.logger.DebugContext(ctx, "115 account throttled", slog.String("component", "adapter"), slog.String("operation", string(r.Operation)), slog.String("error_code", string(Throttled)), slog.Int("upstream_status", resp.StatusCode), slog.Bool("cooldown_blocked", blocked))
		}
		throttle = &Error{Operation: r.Operation, Kind: Throttled, StatusCode: resp.StatusCode, RetryAt: until}
	}
	var inspectionErr error
	if resp.StatusCode == http.StatusOK && readErr == nil && closeErr == nil && int64(len(data)) <= maxBytes && r.Inspect != nil {
		// 业务限流必须在释放账号资格前登记；即使旧凭据响应随后被丢弃，冷却仍属于同一账号。
		inspectionErr = r.Inspect(ctx, data)
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(r.Operation, err)
	}
	// 旧凭据响应不能被采用，但它确认的限流仍属于该稳定账号，必须先保存冷却。
	if r.Check != nil {
		if err := r.Check(ctx); err != nil {
			return nil, checkError(r.Operation, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(r.Operation, err)
	}
	if throttle != nil {
		return nil, throttle
	}
	// 明确的认证和缺失状态不因错误正文截断而重试或切换扫描路径。
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &Error{Operation: r.Operation, Kind: InvalidAuthorization, StatusCode: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, &Error{Operation: r.Operation, Kind: NotFound, StatusCode: resp.StatusCode}
	}
	if int64(len(data)) > maxBytes {
		return nil, &Error{Operation: r.Operation, Kind: Protocol, StatusCode: resp.StatusCode}
	}
	if readErr != nil || closeErr != nil {
		return nil, &Error{Operation: r.Operation, Kind: Temporary, StatusCode: resp.StatusCode}
	}
	if inspectionErr != nil {
		var upstream *Error
		if errors.As(inspectionErr, &upstream) {
			classified := *upstream
			classified.Operation, classified.cause = r.Operation, inspectionErr
			return nil, &classified
		}
		return nil, checkError(r.Operation, inspectionErr)
	}
	kind := Protocol
	switch resp.StatusCode {
	case http.StatusOK:
		return data, nil
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		kind = Temporary
	}
	return nil, &Error{Operation: r.Operation, Kind: kind, StatusCode: resp.StatusCode}
}

// retryAfter 接受整数秒或 HTTP 日期；异常长数值不能被截短为本地默认冷却。
func retryAfter(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64((1<<63-1)/int64(time.Second)) {
			return time.Time{}, true
		}
		return now.Add(time.Duration(seconds) * time.Second), false
	}
	if parsed, err := http.ParseTime(value); err == nil && parsed.After(now) {
		return parsed, false
	}
	return now.Add(30 * time.Second), false
}
