package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestServeAndGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	application, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "app.db"), ShutdownTimeout: time.Second}, slog.New(slog.NewJSONHandler(io.Discard, nil)), nil)
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
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if err := application.database.Ping(t.Context()); err != nil {
		t.Fatalf("database closed before owner cleanup: %v", err)
	}
}
