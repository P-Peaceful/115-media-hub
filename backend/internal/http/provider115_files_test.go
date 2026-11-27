// provider115_files_test.go：验证网盘目录 HTTP 分页转发与上游总数响应，覆盖无效页码以及页码和游标互斥。

package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// cloudCredentialRepo 仅替代目录请求所需的凭据读取，不提供真实存储能力。
type cloudCredentialRepo struct{ provider115.Repository }

// ProviderCredential 提供测试用凭据标识，避免目录浏览测试依赖真实账号。
func (cloudCredentialRepo) ProviderCredential(context.Context) (string, error) {
	return "UID=test", nil
}

// cloudRoundTrip 允许测试捕获并替代 HTTP 传输，测试结束后恢复默认传输器。
type cloudRoundTrip func(*http.Request) (*http.Response, error)

// RoundTrip 委托注入函数模拟上游响应，供测试检查发出的目录查询。
func (f cloudRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestCloudDriveFilesReturnsProviderTotal 验证筛选和页偏移被转发、总数与分页字段被保留，并拒绝非法或互斥分页参数。
func TestCloudDriveFilesReturnsProviderTotal(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = cloudRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("type") != "4" || r.URL.Query().Get("offset") != "1" {
			t.Error("upstream filter was not forwarded")
		}
		body := `{"state":true,"cid":"0","count":7,"path":[{"cid":"0","name":"115网盘"}],"data":[{"fid":"1","n":"movie.mp4"}]}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := providerHandler{service: provider115.New(cloudCredentialRepo{}, slog.New(slog.NewTextHandler(io.Discard, nil)))}
	router.GET("/api/v1/cloud-drive/files", h.files)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/cloud-drive/files?type=video&limit=1&page=2", nil))
	var body struct {
		Total      *int64  `json:"total"`
		Page       *int64  `json:"page"`
		PageSize   int     `json:"page_size"`
		TotalPages int64   `json:"total_pages"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != 200 || body.Total == nil || *body.Total != 7 || body.Page == nil || *body.Page != 2 || body.PageSize != 1 || body.TotalPages != 7 || body.NextCursor == nil {
		t.Fatalf("response: status=%d body=%s err=%v", recorder.Code, recorder.Body.String(), err)
	}
	for _, query := range []string{"page=0", "page=-1", "page=abc", "page=2&cursor=abc", "page=2&cursor=", "page=9007199254740992&limit=1"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/cloud-drive/files?"+query, nil))
		if recorder.Code != 400 {
			t.Errorf("query %s: status %d", query, recorder.Code)
		}
	}

}

// TestProviderFailureDoesNotFabricate503ForCanceledRequest 验证客户端取消 115 请求时不补发 503 响应。
func TestProviderFailureDoesNotFabricate503ForCanceledRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := providerHandler{}
	router.GET("/api/v1/settings/115/details", func(c *gin.Context) {
		h.failure(c, errors.Join(provider115.ErrUpstream, context.Canceled))
	})

	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(requestContext, http.MethodGet, "/api/v1/settings/115/details", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("canceled request fabricated response: status=%d body=%s", response.Code, response.Body.String())
	}
}
