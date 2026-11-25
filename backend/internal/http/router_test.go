// router_test.go：验证健康接口、请求标识、错误脱敏、前端回退及 panic 恢复，确保路由边界不被静态页面吞没。

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
)

// readinessStub 以可注入错误替代数据库探测，隔离健康接口测试。
type readinessStub struct{ err error }

// Ping 返回注入的健康检查错误，模拟数据库就绪与故障。
func (r readinessStub) Ping(context.Context) error { return r.err }

// TestHealthAndErrors 验证存活、就绪、方法错误和未知 API 的状态及错误合同，避免数据库诊断泄露和健康日志刷屏。
func TestHealthAndErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		path    string
		method  string
		failure error
		status  int
		code    generated.APIErrorCode
	}{
		{name: "live", path: "/health/live", method: "GET", status: 200},
		{name: "ready", path: "/health/ready", method: "GET", status: 200},
		{name: "live independent of database", path: "/health/live", method: "GET", failure: errors.New("secret database diagnostic"), status: 200},
		{name: "not ready", path: "/health/ready", method: "GET", failure: errors.New("secret database diagnostic"), status: 503, code: generated.ServiceUnavailable},
		{name: "wrong method", path: "/health/live", method: "POST", status: 405, code: generated.MethodNotAllowed},
		{name: "unknown API", path: "/api/v1/missing", method: "GET", status: 404, code: generated.NotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			router, err := NewRouter(readinessStub{test.failure}, slog.New(slog.NewJSONHandler(&logs, nil)), nil)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("X-Request-ID", "test-request")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			if response.Header().Get("X-Request-ID") != "test-request" {
				t.Fatal("request ID lost")
			}
			if test.code != "" {
				var body generated.ErrorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error.Code != test.code || body.Error.RequestId != "test-request" {
					t.Fatalf("wrong envelope: %+v", body)
				}
			}
			if strings.Contains(response.Body.String()+logs.String(), "secret") {
				t.Fatal("database detail leaked")
			}
			if test.status == 200 && strings.Contains(logs.String(), "http request") {
				t.Fatal("health probes flooded INFO logs")
			}
		})
	}
}

// TestSystemStatusWithoutAuthService 验证未装配认证服务时系统状态仍受保护并禁止缓存。
func TestSystemStatusWithoutAuthService(t *testing.T) {
	router, err := NewRouter(readinessStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil))
	if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, cache control %q: %s", response.Code, response.Header().Get("Cache-Control"), response.Body)
	}
}

// TestRequestIDsAndSafeAccessLogs 验证无效请求标识被替换，访问日志保留关联信息且不记录查询或认证头。
func TestRequestIDsAndSafeAccessLogs(t *testing.T) {
	for _, requestID := range []string{"", strings.Repeat("a", 65), "invalid request", "bad/identifier"} {
		var logs bytes.Buffer
		router, err := NewRouter(readinessStub{}, slog.New(slog.NewJSONHandler(&logs, nil)), nil)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("GET", "/api/missing?token=secret", nil)
		request.Header.Set("X-Request-ID", requestID)
		request.Header.Set("Authorization", "secret")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		actual := response.Header().Get("X-Request-ID")
		if !validRequestID(actual) || actual == requestID {
			t.Fatalf("invalid replacement: %q", actual)
		}
		if strings.Contains(logs.String(), "secret") {
			t.Fatal("access log exposed query or authorization")
		}
		var entry map[string]any
		if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["request_id"] != actual || entry["module"] != "http" || entry["duration_ms"] == nil {
			t.Fatalf("missing correlation: %v", entry)
		}
	}
}

// TestSPAFallbackBoundaries 验证普通页面回退首页，保留 API、健康、播放、缺失资源与隐藏文件的错误边界。
func TestSPAFallbackBoundaries(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<h1>application</h1>")}, "assets/app.js": {Data: []byte("console.log('asset')")}}
	router, err := NewRouter(readinessStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)), assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path    string
		status  int
		content string
	}{
		{"/", 200, "application"}, {"/libraries/123", 200, "application"}, {"/assets/app.js", 200, "console.log"},
		{"/foo/../api/v1/missing", 404, "not_found"}, {"//api/v1/missing", 404, "not_found"}, {"/api/v1/missing", 404, "not_found"}, {"/health/missing", 404, "not_found"}, {"/p/missing", 404, "not_found"},
		{"/assets/missing", 404, "not_found"}, {"/assets", 404, "not_found"}, {"/missing.js", 404, "not_found"}, {"/.env", 404, "not_found"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.content) {
				t.Fatalf("response %d: %s", response.Code, response.Body)
			}
		})
	}
}

// TestRecoveryDoesNotExposePanic 验证未处理 panic 转换为 500，日志和响应均不暴露 panic 原文。
func TestRecoveryDoesNotExposePanic(t *testing.T) {
	var logs bytes.Buffer
	router, err := NewRouter(readinessStub{}, slog.New(slog.NewJSONHandler(&logs, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	router.(*gin.Engine).GET("/panic", func(*gin.Context) { panic("secret panic") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != 500 || strings.Contains(response.Body.String()+logs.String(), "secret") {
		t.Fatalf("unsafe recovery: %s %s", response.Body, &logs)
	}
}
