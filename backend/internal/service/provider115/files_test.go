package provider115

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilesPaginationAndProviderFailures(t *testing.T) {
	response := `{"state":true,"cid":"123","count":2,"path":[{"cid":"0","name":"115网盘"},{"cid":"123","name":"Movies"}],"data":[{"fid":"999999999999999999","cid":"123","n":"movie.mp4","s":12345,"te":"1700000000"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "UID=private" || r.URL.Query().Get("cur") != "1" || r.URL.Query().Get("custom_order") != "1" || r.URL.Query().Get("type") != "4" {
			t.Error("incorrect upstream request")
		}
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	repo := &detailsRepository{credential: "UID=private"}
	logs := &strings.Builder{}
	service := New(repo, slog.New(slog.NewJSONHandler(logs, nil)))
	service.filesURL = server.URL
	query := FileQuery{DirectoryID: "123", Limit: 1, Sort: "modified_at", Order: "desc", Type: "video"}
	page, err := service.Files(t.Context(), query, "request")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "999999999999999999" || page.Items[0].SizeBytes == nil || *page.Items[0].SizeBytes != "12345" || page.Items[0].ModifiedAt == nil || page.NextCursor == nil || len(page.Path) != 2 {
		t.Fatalf("page: %+v, %v", page, err)
	}
	query.Cursor = *page.NextCursor
	response = `{"state":true,"cid":"123","count":2,"path":[{"cid":"0","name":"115网盘"},{"cid":"123","name":"Movies"}],"data":[{"cid":"456","n":"Folder","te":"1700000000"}]}`
	page, err = service.Files(t.Context(), query, "request")
	if err != nil || page.NextCursor != nil || len(page.Items) != 1 || !page.Items[0].IsDirectory {
		t.Fatalf("second page: %+v, %v", page, err)
	}
	query.Sort = "name"
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("cursor mismatch: %v", err)
	}
	query.Cursor = ""
	response = `{"state":true,"cid":"0","count":0,"path":[],"data":[]}`
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrDirectoryNotFound) {
		t.Fatalf("root fallback: %v", err)
	}
	response = `{"state":true,"cid":"123","count":2,"path":[{"cid":"123","name":"Movies"}],"data":[]}`
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("empty incomplete page: %v", err)
	}
	response = `{"state":true,"cid":"123","count":0,"path":[{"cid":"123","name":"Movies"}],"data":[]}`
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("missing root path: %v", err)
	}
	response = `{"state":false,"error":"请先登录"}`
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrInvalidAuthorization) {
		t.Fatalf("invalid authorization: %v", err)
	}
	response = `{"state":false,"error":"系统错误"}`
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("upstream: %v", err)
	}
	repo.credential = ""
	if _, err = service.Files(t.Context(), query, "request"); !errors.Is(err, ErrUnconfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	if strings.Contains(logs.String(), "UID=private") {
		t.Fatal("credential in logs")
	}
}

func TestFilesRejectsAccountReplacement(t *testing.T) {
	for _, replacement := range []string{"", "UID=new"} {
		t.Run(replacement, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(entered)
				<-release
				_, _ = io.WriteString(w, `{"state":true,"cid":"0","count":0,"path":[{"cid":"0","name":"115网盘"}],"data":[]}`)
			}))
			defer server.Close()
			repo := &changingCredentialRepository{}
			repo.credential.Store("UID=old")
			service := New(repo, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			service.filesURL = server.URL
			done := make(chan error, 1)
			go func() {
				_, err := service.Files(t.Context(), FileQuery{DirectoryID: "0", Limit: 50, Sort: "name", Order: "asc", Type: "all"}, "req")
				done <- err
			}()
			<-entered
			repo.credential.Store(replacement)
			close(release)
			if err := <-done; !errors.Is(err, ErrUpstream) {
				t.Fatalf("stale result: %v", err)
			}
		})
	}
}
