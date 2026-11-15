package provider115

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "999999999999999999" || page.Items[0].SizeBytes == nil || *page.Items[0].SizeBytes != "12345" || page.Items[0].ModifiedAt == nil || page.NextCursor == nil || len(page.Path) != 2 || page.Total != 2 {
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

func TestFilesRejectsInvalidCount(t *testing.T) {
	for _, count := range []string{"", `,"count":null`, `,"count":-1`, `,"count":1.5`, `,"count":"2"`, `,"count":9007199254740992`} {
		t.Run(count, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"state":true,"cid":"0","path":[{"cid":"0","name":"115网盘"}],"data":[]`+count+`}`)
			}))
			defer server.Close()
			service := New(&detailsRepository{credential: "UID=private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			service.filesURL = server.URL
			_, err := service.Files(t.Context(), FileQuery{DirectoryID: "0", Limit: 50, Sort: "name", Order: "asc", Type: "all"}, "req")
			if !errors.Is(err, ErrUpstream) {
				t.Fatalf("invalid count %q: %v", count, err)
			}
		})
	}
}

func TestFilesAcceptsZeroCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"state":true,"cid":"0","count":0,"path":[{"cid":"0","name":"115网盘"}],"data":[]}`)
	}))
	defer server.Close()
	service := New(&detailsRepository{credential: "UID=private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.filesURL = server.URL
	page, err := service.Files(t.Context(), FileQuery{DirectoryID: "0", Limit: 50, Sort: "name", Order: "asc", Type: "all"}, "req")
	if err != nil || page.Total != 0 || page.NextCursor != nil || len(page.Items) != 0 {
		t.Fatalf("empty page: %+v %v", page, err)
	}
}

func TestFilesDirectPageAndShortPage(t *testing.T) {
	short := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "50" {
			t.Errorf("offset %s", r.URL.Query().Get("offset"))
		}
		items := make([]map[string]string, 50)
		if short {
			items = items[:1]
		}
		for i := range items {
			items[i] = map[string]string{"fid": fmt.Sprint(i + 51), "n": fmt.Sprintf("%d.mp4", i)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"state": true, "cid": "0", "count": 101, "path": []map[string]string{{"cid": "0", "name": "115网盘"}}, "data": items})
	}))
	defer server.Close()
	service := New(&detailsRepository{credential: "UID=private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.filesURL = server.URL
	q := FileQuery{DirectoryID: "0", Limit: 50, Page: 2, Sort: "name", Order: "asc", Type: "all"}
	result, err := service.Files(t.Context(), q, "req")
	if err != nil || result.Page == nil || *result.Page != 2 || result.PageSize != 50 || result.TotalPages != 3 || len(result.Items) != 50 {
		t.Fatalf("page %+v %v", result, err)
	}
	short = true
	if _, err = service.Files(t.Context(), q, "req"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("short page: %v", err)
	}
}

func TestFilesPagePastEndAfterDirectoryShrinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "150" {
			t.Errorf("unexpected offset %s", r.URL.Query().Get("offset"))
		}
		_, _ = io.WriteString(w, `{"state":true,"cid":"0","count":101,"path":[{"cid":"0","name":"115网盘"}],"data":[]}`)
	}))
	defer server.Close()
	service := New(&detailsRepository{credential: "UID=private"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	service.filesURL = server.URL
	result, err := service.Files(t.Context(), FileQuery{DirectoryID: "0", Limit: 50, Page: 4, Sort: "name", Order: "asc", Type: "all"}, "req")
	if err != nil || result.Page == nil || *result.Page != 4 || result.TotalPages != 3 || len(result.Items) != 0 {
		t.Fatalf("page %+v %v", result, err)
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
