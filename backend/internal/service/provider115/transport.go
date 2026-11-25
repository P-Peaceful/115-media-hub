// transport.go：将账号凭据校验和业务错误映射接入独立 115 传输层，保持服务范围内共享调度与请求预算。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// ErrCredentialChanged 表示在途凭据已替换，仍映射既有上游不可用合同。
var ErrCredentialChanged = errors.Join(ErrUpstream, errors.New("115 credential changed"))

// RequestBudget 供抽样调用方限制实际目录请求（包括重试）；计数由传输发送边界消费。
type RequestBudget = adapter.Budget

// ErrRequestBudgetExhausted 表示本轮抽样的实际请求额度已用尽。
var ErrRequestBudgetExhausted = adapter.ErrBudgetExhausted

// NewRequestBudget 建立一次抽样专用的请求计数器。
func NewRequestBudget(n int) *RequestBudget { return adapter.NewBudget(n) }

// WithRequestBudget 将请求额度传入目录调用及其所有底层重试。
func WithRequestBudget(ctx context.Context, b *RequestBudget) context.Context {
	return adapter.WithBudget(ctx, b)
}

// doRequest 首次固定传输依赖，后续调用共用客户端，避免替换 Cookie 重置已存在冷却。
func (s *Service) doRequest(ctx context.Context, request adapter.Request) ([]byte, error) {
	s.adapterOnce.Do(func() {
		var endpoints []string
		for _, address := range []string{s.baseURL, s.navURL, s.spaceURL, s.filesURL, s.playbackURL, s.categoryURL, s.directoriesURL} {
			u, err := url.Parse(address)
			if err == nil && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") {
				endpoints = append(endpoints, u.Scheme+"://"+u.Host)
			}
		}
		s.adapterClient, s.adapterErr = adapter.New(adapter.Config{Transport: s.client.Transport, Interval: s.requestInterval, TestEndpoints: endpoints, Logger: s.logger})
	})
	if s.adapterErr != nil {
		return nil, ErrUpstream
	}
	return s.adapterClient.Do(ctx, request)
}

// requestHeaders 生成协议头，显式空 UA 用于保持播放器签发语义。
func requestHeaders(cookie, userAgent string) http.Header {
	h := make(http.Header)
	h.Set("User-Agent", userAgent)
	h.Set("Referer", "https://115.com/")
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	return h
}

// accountScope 使用稳定 UID 作为内存账号键；无法识别的遗留凭据仍共用一个保守配额。
func accountScope(cookie string) string {
	if id := credentialAccountID(cookie); id != "" {
		return "account:" + id
	}
	return "single_account"
}

// checkCredential 在每次尝试发送前及采用响应前检查快照，禁止重试切换账号或返回旧授权结果。
func (s *Service) checkCredential(cookie string) func(context.Context) error {
	return func(ctx context.Context) error {
		repo, ok := s.repo.(credentialRepository)
		if !ok {
			return ErrUpstream
		}
		current, err := repo.ProviderCredential(ctx)
		if err != nil {
			return errors.Join(ErrUpstream, err)
		}
		if current != cookie {
			return ErrCredentialChanged
		}
		return nil
	}
}

// readRequest 根据交互或后台性质提交只读协议，目录的大页响应独立限制为 4 MiB。
func (s *Service) readRequest(ctx context.Context, address, cookie string, background, files bool) ([]byte, error) {
	op, priority, maxBytes := adapter.Metadata, adapter.Foreground, int64(1<<20)
	if files {
		op = adapter.Files
		maxBytes = 1 << 20
		if background {
			op = adapter.FastFiles
			maxBytes = 4 << 20
		}
	}
	if background {
		priority = adapter.Background
	}
	data, err := s.doRequest(ctx, adapter.Request{Scope: accountScope(cookie), Priority: priority, Operation: op, Method: http.MethodGet, URL: address, Header: requestHeaders(cookie, "Mozilla/5.0 115Browser/36.0.0"), MaxBytes: maxBytes, Check: s.checkCredential(cookie), Inspect: s.inspectBusiness(cookie)})
	return data, err
}

// mapRequestError 保留取消和安全底层分类，同时映射既有公开业务错误。
func mapRequestError(err, notFound error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCredentialChanged) {
		return err
	}
	var upstream *adapter.Error
	if errors.As(err, &upstream) {
		switch upstream.Kind {
		case adapter.InvalidAuthorization:
			return errors.Join(ErrInvalidAuthorization, err)
		case adapter.NotFound:
			return errors.Join(notFound, err)
		case adapter.Throttled:
			return errors.Join(ErrUpstream, errPlaybackThrottled, err)
		}
	}
	if errors.Is(err, ErrInvalidAuthorization) {
		return err
	}
	return errors.Join(ErrUpstream, err)
}

// businessError 识别有依据的业务授权与限流响应；未知错误留给调用协议完整性判断。
func (s *Service) businessError(data []byte, cookie string) error {
	var envelope struct {
		State    *upstreamState  `json:"state"`
		Code     json.RawMessage `json:"errno"`
		Fallback json.RawMessage `json:"errcode"`
		Error    string          `json:"error"`
		Message  string          `json:"msg"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return nil
	}
	if envelope.State != nil && bool(*envelope.State) {
		return nil
	}
	code := parseInteger(envelope.Code)
	if code == nil {
		code = parseInteger(envelope.Fallback)
	}
	if code != nil {
		switch *code {
		case 99, 990001:
			return ErrInvalidAuthorization
		case 590075:
			until := s.adapterClient.ObserveThrottle(accountScope(cookie), "")
			return errors.Join(ErrUpstream, errPlaybackThrottled, &adapter.Error{Kind: adapter.Throttled, StatusCode: 200, RetryAt: until})
		}
	}
	text := envelope.Error + " " + envelope.Message
	if strings.Contains(text, "请先登录") || strings.Contains(text, "请重新登录") || strings.Contains(text, "未登录") {
		return ErrInvalidAuthorization
	}
	return nil
}

// RequestBudgetFrom 读取抽样传输预算，供协议边界模拟器验证实际尝试次数。
func RequestBudgetFrom(ctx context.Context) *RequestBudget { return adapter.BudgetFromContext(ctx) }

// inspectBusiness 在账号配额释放前检查已知业务错误，使 HTTP 200 限流也立即约束同账号后续请求。
func (s *Service) inspectBusiness(cookie string) func(context.Context, []byte) error {
	return func(_ context.Context, data []byte) error { return s.businessError(data, cookie) }
}
