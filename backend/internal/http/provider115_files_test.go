package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

type cloudCredentialRepo struct{ provider115.Repository }

func (cloudCredentialRepo) ProviderCredential(context.Context) (string, error) {
	return "UID=test", nil
}

type cloudRoundTrip func(*http.Request) (*http.Response, error)

func (f cloudRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
