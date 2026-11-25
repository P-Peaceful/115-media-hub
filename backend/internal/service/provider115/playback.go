// playback.go：负责 Cookie 下载取链协议、响应解码及目标校验，按登记源身份解析临时播放地址。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SheltonZhu/115driver/pkg/crypto/m115"
)

// ErrSourceNotFound 表示 115 已确认找不到登记对应的源文件。
var ErrSourceNotFound = errors.New("115 source file not found")

// errPlaybackThrottled 标记取链遭到上游限流，供错误分类和脱敏日志使用。
var errPlaybackThrottled = errors.New("115 playback lookup throttled")

// playbackTimeout 限制单次播放解析的总时长。
const playbackTimeout = 10 * time.Second

// ResolvePlayback 用当前 115 授权为登记的源文件签发临时链接。
func (s *Service) ResolvePlayback(ctx context.Context, accountID, sourceFileID, playbackCode, userAgent, requestID string) (target string, resultErr error) {
	start := time.Now()
	defer func() { s.logPlayback(ctx, sourceFileID, requestID, start, resultErr) }()
	if accountID == "" || sourceFileID == "" || playbackCode == "" {
		return "", ErrSourceNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()
	repo, ok := s.repo.(credentialRepository)
	if !ok {
		return "", ErrUpstream
	}
	cookie, err := repo.ProviderCredential(ctx)
	if err != nil {
		return "", ErrUpstream
	}
	if cookie == "" {
		return "", ErrUnconfigured
	}
	if credentialAccountID(cookie) != accountID {
		return "", ErrInvalidAuthorization
	}
	target, err = s.requestPlayback(ctx, cookie, sourceFileID, playbackCode, userAgent)
	if err != nil {
		return "", err
	}
	// 查询期间若已解绑或替换凭据，即使上游成功也不能下发旧账号的临时链接。
	current, err := repo.ProviderCredential(ctx)
	if err != nil || current != cookie {
		return "", ErrUpstream
	}
	return target, nil
}

// requestPlayback 编码下载取链请求并透传播放器 UA；禁用带 Cookie 请求的跳转，分类处理认证和限流失败。
func (s *Service) requestPlayback(ctx context.Context, cookie, sourceFileID, playbackCode, userAgent string) (string, error) {
	payload, err := json.Marshal(struct {
		PickCode string `json:"pickcode"`
	}{PickCode: playbackCode})
	if err != nil {
		return "", ErrUpstream
	}
	// Cookie 客户端的下载接口使用 115 自有的请求编码，TLS 仍由标准 HTTP 客户端校验。
	key := m115.GenerateKey()
	form := url.Values{"data": {m115.Encode(payload, key)}}
	query := url.Values{"t": {strconv.FormatInt(s.now().Unix(), 10)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.playbackURL+"?"+query.Encode(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrUpstream
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", "https://115.com/")
	// CDN 可能会把链接绑定到签发时的 UA。空值也显式保留以禁用 Go 默认 UA。
	req.Header.Set("User-Agent", userAgent)
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrUpstream
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.DebugContext(ctx, "115 playback response body close failed", "operation", "resolve_playback")
		}
	}()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errors.Join(ErrUpstream, errPlaybackThrottled)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrInvalidAuthorization
	}
	if resp.StatusCode != http.StatusOK {
		return "", ErrUpstream
	}
	return readPlaybackResponse(resp.Body, key, sourceFileID, playbackCode)
}

// readPlaybackResponse 有界读取取链响应，先判断错误信封，再解码并核对源文件和可信目标。
func readPlaybackResponse(body io.Reader, key m115.Key, sourceFileID, playbackCode string) (string, error) {
	const maxResponse = 64 << 10
	data, err := io.ReadAll(io.LimitReader(body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return "", ErrUpstream
	}
	// 错误响应的 data 可能是空数组，先读取状态和错误语义。
	var envelope struct {
		State   upstreamState   `json:"state"`
		Code    json.RawMessage `json:"errno"`
		ErrCode json.RawMessage `json:"errcode"`
		Error   string          `json:"error"`
		Message string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return "", ErrUpstream
	}
	if !bool(envelope.State) {
		return "", playbackUpstreamError(envelope.Code, envelope.ErrCode, envelope.Error, envelope.Message)
	}
	var encoded string
	if json.Unmarshal(envelope.Data, &encoded) != nil || encoded == "" {
		return "", ErrUpstream
	}
	decoded, err := decodePlaybackData(encoded, key)
	if err != nil {
		return "", err
	}
	return playbackTarget(decoded, sourceFileID, playbackCode)
}

// decodePlaybackData 解码上游下载协议，并将第三方解码器处理畸形数据的 panic 收敛为上游错误。
func decodePlaybackData(encoded string, key m115.Key) (_ []byte, resultErr error) {
	// 第三方解码器假设解包后至少有一个密钥头。把畸形上游数据限制在适配器边界。
	defer func() {
		if recover() != nil {
			resultErr = ErrUpstream
		}
	}()
	decoded, err := m115.Decode(encoded, key)
	if err != nil {
		return nil, ErrUpstream
	}
	return decoded, nil
}

// playbackUpstreamError 将已知上游错误码和登录、文件缺失提示转换为稳定领域错误。
func playbackUpstreamError(code, fallback json.RawMessage, description, message string) error {
	status := parseInteger(code)
	if status == nil {
		status = parseInteger(fallback)
	}
	if status != nil {
		switch *status {
		case 99, 990001:
			return ErrInvalidAuthorization
		case 20018, 31003, 50003, 50015, 70005, 70008, 90008, 430004:
			return ErrSourceNotFound
		case 590075:
			return errors.Join(ErrUpstream, errPlaybackThrottled)
		}
	}
	description += " " + message
	if strings.Contains(description, "请先登录") || strings.Contains(description, "请重新登录") || strings.Contains(description, "未登录") {
		return ErrInvalidAuthorization
	}
	if strings.Contains(description, "文件不存在") || strings.Contains(description, "文件已删除") {
		return ErrSourceNotFound
	}
	return ErrUpstream
}

// playbackTarget 按源文件 ID 和可选定位码核对响应，仅返回通过域名与 URL 结构校验的下载目标。
func playbackTarget(data []byte, sourceFileID, playbackCode string) (string, error) {
	var files map[string]struct {
		PickCode string          `json:"pick_code"`
		URL      json.RawMessage `json:"url"`
	}
	if json.Unmarshal(data, &files) != nil {
		return "", ErrUpstream
	}
	file, found := files[sourceFileID]
	if !found || (file.PickCode != "" && file.PickCode != playbackCode) {
		return "", ErrSourceNotFound
	}
	var target struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(file.URL, &target) != nil || !validPlaybackURL(target.URL) {
		return "", ErrUpstream
	}
	return target.URL, nil
}

// validPlaybackURL 只允许无用户信息、片段或显式端口的 HTTP(S) 115 域名链接。
func validPlaybackURL(raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") || target.Opaque != "" || target.User != nil || target.Fragment != "" || target.Hostname() == "" || target.Path == "" {
		return false
	}
	host := strings.ToLower(target.Hostname())
	if target.Host != target.Hostname() {
		return false
	}
	// 最终地址只交给播放器，且必须属于 115 的域名边界。
	return host == "115.com" || strings.HasSuffix(host, ".115.com") ||
		host == "115cdn.com" || strings.HasSuffix(host, ".115cdn.com") ||
		host == "115cdn.net" || strings.HasSuffix(host, ".115cdn.net")
}

// logPlayback 记录取链的稳定错误类别和源文件 ID，不记录 Cookie、UA、定位码或临时链接。
func (s *Service) logPlayback(ctx context.Context, sourceFileID, requestID string, start time.Time, err error) {
	attrs := []any{"operation", "resolve_playback", "request_id", requestID, "duration_ms", time.Since(start).Milliseconds()}
	if sourceFileID != "" {
		attrs = append(attrs, "source_file_id", sourceFileID)
	}
	if err == nil {
		s.logger.DebugContext(ctx, "115 playback link resolved", append(attrs, "outcome", "succeeded")...)
		return
	}
	outcome, errorCode := "failed", "provider_115_failed"
	switch {
	case errors.Is(err, ErrSourceNotFound):
		errorCode = "source_not_found"
	case errors.Is(err, ErrUnconfigured):
		errorCode = "provider_not_configured"
	case errors.Is(err, ErrInvalidAuthorization):
		errorCode = "provider_auth_invalid"
	case errors.Is(err, errPlaybackThrottled):
		errorCode = "provider_throttled"
	case errors.Is(err, context.Canceled):
		outcome, errorCode = "canceled", "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		errorCode = "provider_timeout"
	}
	s.logger.WarnContext(ctx, "115 playback link unavailable", append(attrs, "outcome", outcome, "error_code", errorCode)...)
}
