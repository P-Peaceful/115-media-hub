package provider115

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testRepository struct {
	cookie string
	name   string
	fail   bool
}

func (r *testRepository) ProviderAccount(context.Context) (Account, error) {
	return Account{Connected: r.cookie != ""}, nil
}
func (r *testRepository) SaveProviderAccount(_ context.Context, name, cookie string, _ time.Time, _, _, _ string) error {
	if r.fail {
		return errors.New("storage")
	}
	r.name = name
	r.cookie = cookie
	return nil
}
func (r *testRepository) DeleteProviderAccount(context.Context, string, string, time.Time) error {
	r.cookie = ""
	return nil
}
func TestQRWorkflow(t *testing.T) {
	status := 0
	login := `{"state":1,"data":{"user_name":"Alice","cookie":{"UID":"u","CID":"c","SEID":"s"}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/1.0/web/1.0/token/":
			_, _ = io.WriteString(w, `{"state":1,"data":{"uid":"secret","time":1,"sign":"secret"}}`)
		case "/api/1.0/mac/1.0/qrcode":
			_, _ = io.WriteString(w, "png")
		case "/get/status/":
			_, _ = io.WriteString(w, `{"state":1,"data":{"status":`+string(rune('0'+status))+`}}`)
		case "/app/1.0/alipaymini/1.0/login/qrcode/":
			_, _ = io.WriteString(w, login)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	repo := &testRepository{cookie: "old"}
	logs := &strings.Builder{}
	service := New(repo, slog.New(slog.NewJSONHandler(logs, nil)))
	service.baseURL = server.URL
	ctx := context.Background()
	qr, err := service.Start(ctx, "owner", "req")
	if err != nil || !strings.HasPrefix(qr.QRURL, "data:image/png;base64,") {
		t.Fatalf("start: %v", err)
	}
	if _, err = service.Poll(ctx, "other", qr.ID, "actor", "req"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner isolation: %v", err)
	}
	service.InvalidateSession("owner")
	if _, err = service.Poll(ctx, "owner", qr.ID, "actor", "req"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("logged-out session retained QR state: %v", err)
	}
	qr, err = service.Start(ctx, "owner", "req")
	if err != nil {
		t.Fatal(err)
	}
	status = 1
	qr, err = service.Poll(ctx, "owner", qr.ID, "actor", "req")
	if err != nil || qr.Status != "scanned" {
		t.Fatalf("scanned: %+v %v", qr, err)
	}
	status = 2
	login = `{"state":1,"data":{"cookie":{"UID":"u","CID":"c","KID":"k"}}}`
	if _, err = service.Poll(ctx, "owner", qr.ID, "actor", "req"); !errors.Is(err, ErrUpstream) || repo.cookie != "old" {
		t.Fatal("invalid upstream must preserve existing authorization")
	}
	login = `{"state":1,"data":{"user_name":"Alice","cookie":{"UID":"u","CID":"c","SEID":"s"}}}`
	qr, err = service.Poll(ctx, "owner", qr.ID, "actor", "req")
	if err != nil || qr.Status != "succeeded" || repo.name != "Alice" || repo.cookie != "UID=u; CID=c; SEID=s" {
		t.Fatalf("success: %+v %v", qr, err)
	}
	if strings.Contains(logs.String(), "secret") || strings.Contains(logs.String(), "UID=u") {
		t.Fatal("secrets leaked to logs")
	}
}

func TestCancelDuringPollingPreventsAuthorization(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/1.0/web/1.0/token/":
			_, _ = io.WriteString(w, `{"state":1,"data":{"uid":"uid","time":1,"sign":"sign"}}`)
		case "/api/1.0/mac/1.0/qrcode":
			_, _ = io.WriteString(w, "png")
		case "/get/status/":
			close(entered)
			<-release
			_, _ = io.WriteString(w, `{"state":1,"data":{"status":2}}`)
		default:
			t.Error("canceled request must not exchange credentials")
		}
	}))
	defer server.Close()
	repo := &testRepository{cookie: "old"}
	service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.baseURL = server.URL
	qr, err := service.Start(t.Context(), "owner", "req")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := service.Poll(t.Context(), "owner", qr.ID, "actor", "req"); done <- err }()
	<-entered
	if err := service.Cancel(t.Context(), "owner", qr.ID, "req"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrNotFound) || repo.cookie != "old" {
		t.Fatalf("cancelled poll changed authorization: %v", err)
	}
}
