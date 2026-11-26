// playback.go：按登记源身份为播放器解析临时地址，定义取链领域错误和共享跳转边界。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// ErrSourceNotFound 表示 115 已确认找不到登记对应的源文件。
var ErrSourceNotFound = errors.New("115 source file not found")

// ErrDownloadLinkResponse 表示 115 取链响应无法解析、解码或通过源身份核验。
var ErrDownloadLinkResponse = errors.New("115 download link response invalid")

// ErrDownloadUntrustedTarget 表示取链得到的目标不在允许的 115 CDN 域名边界内。
var ErrDownloadUntrustedTarget = errors.New("115 download target untrusted")

// ErrDownloadThrottled 表示取链或 CDN 内容请求明确遭到限流。
var ErrDownloadThrottled = errors.New("115 sidecar download throttled")

// ErrDownloadLinkExpired 表示 CDN 明确拒绝了已过期的签名地址。
var ErrDownloadLinkExpired = errors.New("115 sidecar signed link expired")

// errPlaybackThrottled 保留播放服务内部的历史错误身份，后台下载复用同一限流标记。
var errPlaybackThrottled = ErrDownloadThrottled

// playbackTimeout 限制单次播放解析的总时长。
const playbackTimeout = 15 * time.Second

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
		return "", errors.Join(ErrUpstream, err)
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
	if err != nil {
		return "", errors.Join(ErrUpstream, err)
	}
	if current != cookie {
		return "", ErrCredentialChanged
	}
	return target, nil
}

// requestPlayback 透传播放器 UA；只将临时 URL 交给播放出口，不发布取链响应 Cookie。
func (s *Service) requestPlayback(ctx context.Context, cookie, sourceFileID, playbackCode, userAgent string) (string, error) {
	target, err := s.resolveDownloadTarget(ctx, cookie, sourceFileID, playbackCode, userAgent, adapter.Playback, adapter.Foreground)
	return target.URL, err
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
	return errors.Join(ErrDownloadLinkResponse, ErrUpstream)
}

// playbackTarget 按源文件 ID 和可选定位码核对响应，仅返回通过域名与 URL 结构校验的下载目标。
func playbackTarget(data []byte, sourceFileID, playbackCode string) (string, error) {
	var files map[string]struct {
		PickCode string          `json:"pick_code"`
		URL      json.RawMessage `json:"url"`
	}
	if json.Unmarshal(data, &files) != nil {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	file, found := files[sourceFileID]
	if !found || (file.PickCode != "" && file.PickCode != playbackCode) {
		return "", ErrSourceNotFound
	}
	var target struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(file.URL, &target) != nil || !validPlaybackURL(target.URL) {
		return "", errors.Join(ErrDownloadUntrustedTarget, ErrUpstream)
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
