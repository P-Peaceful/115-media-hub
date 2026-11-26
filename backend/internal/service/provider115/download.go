// download.go：负责附属文件的后台取链、流式大小与长度校验、有限重试和凭据快照回执。
// 内容传输不持有账号 API 配额，不负责本地路径、临时文件归属或最终发布。

package provider115

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

const (
	downloadUserAgent      = "Mozilla/5.0 115Browser/36.0.0"
	downloadTimeout        = 120 * time.Second
	downloadAttemptTimeout = 30 * time.Second
)

// ErrDownloadTooLarge 表示源大小或实际内容超出对应类型上限。
var ErrDownloadTooLarge = errors.New("115 sidecar size limit exceeded")

// ErrDownloadLength 表示已知源大小、响应长度或实际读取字节数不一致。
var ErrDownloadLength = errors.New("115 sidecar length mismatch")

// ErrDownloadQueueFull 表示本账号下载的有界等待队列已满。
var ErrDownloadQueueFull = errors.New("115 sidecar download queue full")

// ErrDownloadTakeLink 表示后台下载取链请求或取链业务响应失败。
var ErrDownloadTakeLink = errors.New("115 sidecar take-link failed")

// ErrDownloadContentRequest 表示已取链后建立 CDN 内容请求失败。
var ErrDownloadContentRequest = errors.New("115 sidecar content request failed")

// ErrDownloadContentStatus 表示 CDN 返回非成功 HTTP 状态。
var ErrDownloadContentStatus = errors.New("115 sidecar content status failed")

// DownloadRequest 仅接受已通过同步层资格与输出检查的源身份；不接收外部 URL 或任意头。
// ExpectedBytes 为 nil 时大小未知，非 nil 必须非负；Kind 只接受 nfo/image。
type DownloadRequest struct {
	AccountID, SourceFileID, PlaybackCode, Kind, RequestID string
	ExpectedBytes                                          *int64
}

// DownloadSink 由同步层拥有并关闭；每次尝试先清空和复位，失败内容不能作为完整文件发布。
type DownloadSink interface {
	io.Writer
	io.Seeker
	Truncate(int64) error
}

// DownloadReceipt 保存完整下载的字节数与不公开凭据的发布校验器；调用者必须在发布前调用 Check。
type DownloadReceipt struct {
	Bytes int64
	// CredentialCheck 捕获下载时的完整凭据快照，不得为空；不含可序列化或可输出的凭据材料。
	CredentialCheck func(context.Context) error
}

// Check 再次核对下载时捕获的完整 Cookie 快照；同 UID 重新授权也会使旧回执失效。
func (r DownloadReceipt) Check(ctx context.Context) error {
	if r.CredentialCheck == nil {
		return ErrUpstream
	}
	return r.CredentialCheck(ctx)
}

// downloadError 保留原因的 errors.Is 身份，文本只返回固定错误码，不泄漏签名 URL 或路径。
type downloadError struct {
	code  string
	cause error
}

// Error 返回可记录的固定描述，底层网络错误和磁盘路径不进入文本。
func (e *downloadError) Error() string { return "115 sidecar: " + e.code }

// Unwrap 保留取消、长度、大小和本地写失败的错误身份供调用者分类。
func (e *downloadError) Unwrap() error { return e.cause }

// DownloadSidecar 在总预算内排队、取链并下载到调用方暂存文件；成功不代表文件已发布。
// 同账号最多两项处理中，等待最多十六项；凭据变化会取消 API 等待和内容传输。
func (s *Service) DownloadSidecar(parent context.Context, request DownloadRequest, sink DownloadSink) (receipt DownloadReceipt, resultErr error) {
	start := time.Now()
	defer func() { s.logDownload(parent, request, start, receipt.Bytes, resultErr) }()
	limit := int64(0)
	switch request.Kind {
	case "nfo":
		limit = 8 << 20
	case "image":
		limit = 64 << 20
	default:
		return receipt, ErrInvalidQuery
	}
	if request.AccountID == "" || request.SourceFileID == "" || request.PlaybackCode == "" || sink == nil || (request.ExpectedBytes != nil && *request.ExpectedBytes < 0) {
		return receipt, ErrInvalidQuery
	}
	if request.ExpectedBytes != nil && *request.ExpectedBytes > limit {
		return receipt, ErrDownloadTooLarge
	}
	ctx, cancel := context.WithTimeout(parent, downloadTimeout)
	defer cancel()
	repo, ok := s.repo.(credentialRepository)
	if !ok {
		return receipt, ErrUpstream
	}
	cookie, err := repo.ProviderCredential(ctx)
	if err != nil {
		return receipt, &downloadError{"credential_read", err}
	}
	if cookie == "" {
		return receipt, ErrUnconfigured
	}
	if credentialAccountID(cookie) != request.AccountID {
		return receipt, ErrInvalidAuthorization
	}
	check := s.checkCredential(cookie)
	gate, err := s.acquireDownload(ctx, accountScope(cookie), check)
	if err != nil {
		return receipt, err
	}
	defer s.releaseDownload(gate)
	ctx, stop := monitorDownloadCredential(ctx, check)
	defer stop()
	resolve := s.downloads.resolve
	if resolve == nil {
		resolve = func(ctx context.Context, cookie, source, code string) (string, error) {
			return s.requestDownloadLink(ctx, cookie, source, code, downloadUserAgent, adapter.DownloadLink, adapter.Background)
		}
	}
	target, err := resolve(ctx, cookie, request.SourceFileID, request.PlaybackCode)
	if err != nil {
		err = wrapDownloadTakeLinkError(err)
		return receipt, downloadContextError(ctx, err)
	}
	refreshed := false
	for attempt := 1; attempt <= 3; attempt++ {
		if err = s.waitDownloadCooldown(ctx, gate, check); err != nil {
			return receipt, err
		}
		if !validDownloadURL(target) {
			return receipt, &downloadError{"untrusted_target", errors.Join(ErrDownloadUntrustedTarget, ErrUpstream)}
		}
		if err = resetDownloadSink(sink); err != nil {
			return receipt, err
		}
		var bytes int64
		var retry bool
		bytes, retry, err = s.downloadAttempt(ctx, target, request.ExpectedBytes, limit, sink, check, gate)
		if err == nil {
			if err = check(ctx); err != nil {
				return receipt, err
			}
			return DownloadReceipt{Bytes: bytes, CredentialCheck: check}, nil
		}
		if ctx.Err() != nil {
			return receipt, downloadContextError(ctx, err)
		}
		if errors.Is(err, errDownloadExpired) && !refreshed && attempt < 3 {
			refreshed = true
			target, err = resolve(ctx, cookie, request.SourceFileID, request.PlaybackCode)
			if err != nil {
				err = wrapDownloadTakeLinkError(err)
				return receipt, downloadContextError(ctx, err)
			}
			continue
		}
		if !retry || attempt == 3 {
			return receipt, err
		}
		if err = waitDownload(ctx, time.Duration(attempt)*time.Second); err != nil {
			return receipt, downloadContextError(ctx, err)
		}
	}
	return receipt, ErrUpstream
}

// wrapDownloadTakeLinkError 只给未知取链错误增加阶段标记；授权、限流、取消和已知响应错误保留原始分类。
func wrapDownloadTakeLinkError(err error) error {
	if err == nil || errors.Is(err, ErrSourceNotFound) || errors.Is(err, ErrDownloadLinkResponse) ||
		errors.Is(err, ErrDownloadUntrustedTarget) || errors.Is(err, ErrDownloadThrottled) ||
		errors.Is(err, ErrCredentialChanged) || errors.Is(err, ErrInvalidAuthorization) ||
		errors.Is(err, ErrUnconfigured) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(ErrDownloadTakeLink, err)
}

// resetDownloadSink 丢弃上次失败尝试的所有暂存字节，禁止把重试内容追加到旧半文件。
func resetDownloadSink(sink DownloadSink) error {
	if err := sink.Truncate(0); err != nil {
		return &downloadError{"sink_reset", err}
	}
	if _, err := sink.Seek(0, io.SeekStart); err != nil {
		return &downloadError{"sink_seek", err}
	}
	return nil
}

// validDownloadURL 在播放链接的域边界之上强制 HTTPS；内容请求不携带 Cookie 且逐跳复查。
func validDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && validPlaybackURL(raw)
}

// errDownloadExpired 保留内部过期判定名称，公开错误身份供任务日志分类。
var errDownloadExpired = ErrDownloadLinkExpired

// errDownloadTarget 标记目标边界校验失败，安全拒绝不得作为临时网络错误重试。
var errDownloadTarget = errors.New("115 sidecar untrusted target")

// downloadAttempt 执行一个有界内容 GET；TLS 使用标准验证，每跳可信且最多五跳，不转发 Cookie。
func (s *Service) downloadAttempt(parent context.Context, target string, expected *int64, limit int64, sink DownloadSink, check func(context.Context) error, gate *downloadGate) (int64, bool, error) {
	ctx, cancel := context.WithTimeout(parent, downloadAttemptTimeout)
	defer cancel()
	if err := check(ctx); err != nil {
		return 0, false, err
	}
	client := &http.Client{Transport: s.client.Transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !validDownloadURL(req.URL.String()) {
			return &downloadError{"untrusted_redirect", errors.Join(ErrDownloadUntrustedTarget, errDownloadTarget)}
		}
		req.Header = downloadHeaders()
		return check(req.Context())
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, false, &downloadError{"invalid_target", errors.Join(ErrDownloadUntrustedTarget, ErrUpstream)}
	}
	req.Header = downloadHeaders()
	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}
		return 0, !errors.Is(err, ErrCredentialChanged) && !errors.Is(err, errDownloadTarget), &downloadError{"content_request", errors.Join(ErrDownloadContentRequest, err)}
	}
	// 主动关闭错误响应，不读取或记录其可能含凭据的正文。
	if resp.StatusCode != http.StatusOK {
		closeErr := resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			s.observeDownloadThrottle(gate, resp.Header.Get("Retry-After"))
			return 0, true, &downloadError{"content_throttled", errors.Join(errPlaybackThrottled, ErrDownloadContentStatus, closeErr)}
		}
		if resp.StatusCode == http.StatusForbidden && expiredDownloadURL(target, time.Now()) {
			return 0, false, &downloadError{"content_link_expired", errors.Join(errDownloadExpired, closeErr)}
		}
		retry := resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
		return 0, retry, &downloadError{"content_status", errors.Join(ErrDownloadContentStatus, ErrUpstream, closeErr)}
	}
	if err = check(ctx); err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return 0, false, err
	}
	if resp.ContentLength > limit || (expected != nil && resp.ContentLength >= 0 && resp.ContentLength != *expected) || resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity" {
		cause := ErrDownloadLength
		if resp.ContentLength > limit {
			cause = ErrDownloadTooLarge
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			cause = errors.Join(cause, closeErr)
		}
		return 0, false, &downloadError{"content_length", cause}
	}
	writer := &downloadWriter{ctx: ctx, sink: sink, check: check}
	written, copyErr := io.Copy(writer, io.LimitReader(resp.Body, limit+1))
	closeErr := resp.Body.Close()
	if written > limit {
		return written, false, &downloadError{"size_limit", ErrDownloadTooLarge}
	}
	if copyErr != nil || closeErr != nil {
		cause := errors.Join(copyErr, closeErr)
		if errors.Is(copyErr, io.ErrUnexpectedEOF) {
			cause = errors.Join(ErrDownloadLength, cause)
		}
		retry := writer.err == nil && parent.Err() == nil
		return written, retry, &downloadError{"content_read", cause}
	}
	if expected != nil && written != *expected || resp.ContentLength >= 0 && written != resp.ContentLength {
		return written, true, &downloadError{"content_length", ErrDownloadLength}
	}
	return written, false, nil
}

// downloadHeaders 固定下载上下文，要求原始字节，绝不借用管理员会话、播放器头或网盘 Cookie。
func downloadHeaders() http.Header {
	h := make(http.Header)
	h.Set("User-Agent", downloadUserAgent)
	h.Set("Referer", "https://115.com/")
	h.Set("Accept-Encoding", "identity")
	return h
}

// downloadWriter 在每次落盘前复查凭据和取消，保留本地写错误使其不会被当作网络故障重试。
type downloadWriter struct {
	ctx   context.Context
	sink  io.Writer
	check func(context.Context) error
	err   error
}

// Write 限定一次同步写入的上下文与凭据；文件的关闭和完整性发布由同步层负责。
func (w *downloadWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return 0, err
	}
	if err := w.check(w.ctx); err != nil {
		w.err = err
		return 0, err
	}
	n, err := w.sink.Write(p)
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

// downloadContextError 保留凭据监视器的取消原因；普通取消仍保持原 context 错误身份。
func downloadContextError(ctx context.Context, fallback error) error {
	if cause := context.Cause(ctx); cause != nil {
		return &downloadError{"canceled", cause}
	}
	return fallback
}

// logDownload 在下载边界记录实际类型与字节数，不输出源名称、路径、签名 URL 或上游错误文本。
func (s *Service) logDownload(ctx context.Context, request DownloadRequest, start time.Time, bytes int64, err error) {
	attrs := []any{"operation", "download_sidecar", "request_id", request.RequestID, "source_file_id", request.SourceFileID, "kind", request.Kind, "duration_ms", time.Since(start).Milliseconds(), "bytes", bytes}
	if err == nil {
		s.logger.DebugContext(ctx, "115 sidecar downloaded", append(attrs, "outcome", "succeeded")...)
		return
	}
	code, outcome := "sidecar_download_failed", "failed"
	switch {
	case errors.Is(err, context.Canceled):
		code, outcome = "request_canceled", "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		code = "download_timeout"
	case errors.Is(err, ErrCredentialChanged):
		code = "provider_credential_changed"
	case errors.Is(err, ErrInvalidAuthorization):
		code = "provider_auth_invalid"
	case errors.Is(err, ErrUnconfigured):
		code = "provider_not_configured"
	case errors.Is(err, ErrSourceNotFound):
		code = "sidecar_source_not_found"
	case errors.Is(err, ErrDownloadLinkResponse):
		code = "sidecar_take_link_response_invalid"
	case errors.Is(err, ErrDownloadUntrustedTarget):
		code = "sidecar_untrusted_target"
	case errors.Is(err, errDownloadExpired):
		code = "sidecar_link_expired"
	case errors.Is(err, errPlaybackThrottled):
		code = "download_throttled"
	case errors.Is(err, ErrDownloadTakeLink):
		code = "sidecar_take_link_failed"
	case errors.Is(err, ErrDownloadContentRequest):
		code = "sidecar_content_request_failed"
	case errors.Is(err, ErrDownloadTooLarge):
		code = "download_too_large"
	case errors.Is(err, ErrDownloadLength):
		code = "download_length_mismatch"
	case errors.Is(err, ErrDownloadQueueFull):
		code = "download_queue_full"
	case errors.Is(err, ErrDownloadContentStatus):
		code = "sidecar_content_status_failed"
	}
	s.logger.WarnContext(ctx, "115 sidecar unavailable", append(attrs, "outcome", outcome, "error_code", code)...)
}
