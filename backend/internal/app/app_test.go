// app_test.go：通过真实 HTTP 监听器验证应用就绪、事件流关闭和任务中心停止，同时检查数据库由外层生命周期释放。

package app

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	logdomain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/logs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/auth"
)

// TestServeAndGracefulShutdown 验证取消应用上下文后 HTTP 与 SSE 有界退出、任务中心停止接纳订阅，数据库仍留待调用方关闭。
func TestServeAndGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	databasePath := filepath.Join(t.TempDir(), "app.db")
	application, err := New(ctx, Config{DatabasePath: databasePath, ShutdownTimeout: time.Second}, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := application.Close(); err != nil {
			t.Error(err)
		}
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- application.Serve(ctx, listener) }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("readiness: %d", response.StatusCode)
	}
	authentication, err := auth.New(application.database, logger, auth.Options{PasswordCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	session, err := authentication.Initialize(ctx, "admin", "ValidPass123!", domain.Settings{
		ServiceURL: "https://media.example.com", OutputDirectory: "/app/media", SessionLifetimeDays: 7,
	}, "127.0.0.1", "shutdown-test")
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: "mediahub_session", Value: session.Token})
	streamClient := &http.Client{Timeout: 4 * time.Second}
	stream, err := streamClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("event stream unavailable: %d", stream.StatusCode)
	}
	logRequest := request.Clone(t.Context())
	logRequest.URL.Path = "/api/v1/logs/stream"
	logRequest.URL.RawQuery = "module=upload"
	logStream, err := streamClient.Do(logRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := logStream.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if logStream.StatusCode != http.StatusOK || logStream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("runtime log stream unavailable: %d", logStream.StatusCode)
	}
	logReader := bufio.NewReader(logStream.Body)
	if line, err := logReader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("runtime log stream did not establish: %q, %v", line, err)
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if data, err := io.ReadAll(stream.Body); err != nil || string(data) != ": connected\n\n" {
		t.Fatalf("event stream did not close cleanly: %q, %v", data, err)
	}
	if data, err := io.ReadAll(logReader); err != nil || strings.TrimSpace(string(data)) != "" {
		t.Fatalf("runtime log stream did not close cleanly: %q, %v", data, err)
	}
	if events, _ := application.jobs.Subscribe(); events != nil {
		t.Fatal("task center still accepting subscriptions after shutdown")
	}
	if err := application.database.Ping(t.Context()); err != nil {
		t.Fatalf("database closed before owner cleanup: %v", err)
	}
	reopened, err := sqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	entries, err := reopened.ListRuntimeLogs(t.Context(), logdomain.Filter{Module: "system", Limit: 10})
	if err != nil || len(entries) < 3 || entries[0].Operation != "shutdown" {
		t.Fatalf("startup and shutdown logs were not persisted: %+v, %v", entries, err)
	}
}
