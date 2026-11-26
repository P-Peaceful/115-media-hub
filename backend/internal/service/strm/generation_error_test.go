// generation_error_test.go：验证附属下载错误可映射为稳定任务错误码，不泄露上游正文或地址。

package strm

import (
	"errors"
	"testing"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// TestGenerationErrorClassifiesSidecarFailures 验证取链、内容和限流错误不会退化为 provider_unavailable。
func TestGenerationErrorClassifiesSidecarFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{name: "throttled", err: errors.Join(provider115.ErrDownloadThrottled, provider115.ErrDownloadContentStatus), code: "sidecar_download_throttled"},
		{name: "source missing", err: provider115.ErrSourceNotFound, code: "sidecar_source_not_found"},
		{name: "response", err: errors.Join(provider115.ErrDownloadLinkResponse, provider115.ErrUpstream), code: "sidecar_take_link_response_invalid"},
		{name: "target", err: provider115.ErrDownloadUntrustedTarget, code: "sidecar_untrusted_target"},
		{name: "take link", err: errors.Join(provider115.ErrDownloadTakeLink, provider115.ErrUpstream), code: "sidecar_take_link_failed"},
		{name: "content request", err: errors.Join(provider115.ErrDownloadContentRequest, provider115.ErrUpstream), code: "sidecar_content_request_failed"},
		{name: "content status", err: errors.Join(provider115.ErrDownloadContentStatus, provider115.ErrUpstream), code: "sidecar_content_status_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := generationError(tc.err)
			if code != tc.code {
				t.Fatalf("code=%q, want %q", code, tc.code)
			}
		})
	}
}
