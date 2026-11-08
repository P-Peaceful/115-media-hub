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

type readinessStub struct{ err error }

func (r readinessStub) Ping(context.Context) error { return r.err }

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
