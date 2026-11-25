// library_test.go：通过认证路由验证媒体库 CRUD、启停和分页，并覆盖现存文件策略、CSRF 和版本冲突校验。

package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// TestLibraryManagementBoundaryAndCRUD 验证媒体库权限、完整配置保存与读取、分页、启停、删除，以及非法字段和过期版本拒绝。
func TestLibraryManagementBoundaryAndCRUD(t *testing.T) {
	b, authService, db := newAuthBrowser(t, false, nil)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler, err := NewRouter(db, logger, nil, AuthOptions{Service: authService, Library: library.New(db, logger)})
	if err != nil {
		t.Fatal(err)
	}
	b.handler = handler
	if r := b.request(t, "GET", "/api/v1/libraries", "", nil); r.Code != 401 {
		t.Fatalf("unauthorized: %d", r.Code)
	}
	b.setup(t)
	body := `{"name":"电影","directory_id":"123","directory_path":"/电影","media_type":"movie","output_path":"/media","generate_strm":true,"strm_existing_file_policy":"skip","generate_nfo":true,"generate_poster_cover":true,"language_preference":"zh-CN","scrape_source":"tmdb","exclude_directory_path":"","include_subdirectories":true,"follow_directory_changes":true,"file_encoding":"UTF-8","link_type":"proxy","strm_direct_domain":"","strm_link_format":"original_name"}`
	for _, invalid := range []string{
		strings.Replace(body, `"strm_existing_file_policy":"skip",`, "", 1),
		strings.Replace(body, `"strm_existing_file_policy":"skip"`, `"strm_existing_file_policy":null`, 1),
		strings.Replace(body, `"strm_existing_file_policy":"skip"`, `"strm_existing_file_policy":"rename"`, 1),
		strings.Replace(body, `"strm_existing_file_policy":"skip"`, `"strm_existing_file_policy":true`, 1),
	} {
		if response := b.request(t, "POST", "/api/v1/libraries", invalid, nil); response.Code != 400 {
			t.Fatalf("invalid existing-file policy accepted: %d %s", response.Code, response.Body)
		}
	}
	withEnabled := strings.TrimSuffix(body, "}") + `,"enabled":false}`
	if r := b.request(t, "POST", "/api/v1/libraries", withEnabled, nil); r.Code != 400 {
		t.Fatalf("response-only enabled accepted: %d %s", r.Code, r.Body)
	}
	if r := b.request(t, "POST", "/api/v1/libraries", body, map[string]string{"X-CSRF-Token": "invalid"}); r.Code != 403 {
		t.Fatalf("csrf: %d", r.Code)
	}
	r := b.request(t, "POST", "/api/v1/libraries", body, nil)
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	var item library.Library
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.STRMExistingFilePolicy != "skip" {
		t.Fatal(err)
	}
	if r = b.request(t, "PATCH", "/api/v1/libraries/"+item.ID+"/enabled", `{"enabled":false}`, nil); r.Code != 200 {
		t.Fatalf("pause: %d %s", r.Code, r.Body)
	}
	if r = b.request(t, "GET", "/api/v1/libraries/"+item.ID, "", nil); r.Code != 200 {
		t.Fatalf("get: %d", r.Code)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.Enabled {
		t.Fatalf("stored enabled: %+v %v", item, err)
	}
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, body, map[string]string{"X-Library-Revision": "1"}); r.Code != 409 {
		t.Fatalf("stale edit: %d %s", r.Code, r.Body)
	}
	invalidUpdate := strings.Replace(body, `"strm_existing_file_policy":"skip"`, `"strm_existing_file_policy":"rename"`, 1)
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, invalidUpdate, map[string]string{"X-Library-Revision": "2"}); r.Code != 400 {
		t.Fatalf("invalid update policy accepted: %d %s", r.Code, r.Body)
	}
	updatedBody := strings.Replace(body, `"scrape_source":"tmdb"`, `"scrape_source":"metatube"`, 1)
	updatedBody = strings.Replace(updatedBody, `"strm_existing_file_policy":"skip"`, `"strm_existing_file_policy":"overwrite"`, 1)
	updatedBody = strings.Replace(updatedBody, `"generate_strm":true`, `"generate_strm":false`, 1)
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, updatedBody, nil); r.Code != 400 {
		t.Fatalf("missing revision: %d", r.Code)
	}
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, updatedBody, map[string]string{"X-Library-Revision": "2"}); r.Code != 200 {
		t.Fatalf("update: %d %s", r.Code, r.Body)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.Revision != 3 || item.ScrapeSource != "metatube" || item.Enabled || item.STRMExistingFilePolicy != "overwrite" || item.GenerateSTRM {
		t.Fatalf("updated: %+v %v", item, err)
	}
	if r = b.request(t, "GET", "/api/v1/libraries/"+item.ID, "", nil); r.Code != 200 {
		t.Fatalf("get updated: %d", r.Code)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.Revision != 3 || item.ScrapeSource != "metatube" || item.Enabled || item.STRMExistingFilePolicy != "overwrite" || item.GenerateSTRM {
		t.Fatalf("round trip: %+v %v", item, err)
	}
	if r = b.request(t, "POST", "/api/v1/libraries", body, nil); r.Code != 201 {
		t.Fatalf("second create: %d", r.Code)
	}
	if r = b.request(t, "GET", "/api/v1/libraries?limit=1", "", nil); r.Code != 200 {
		t.Fatalf("first page: %d", r.Code)
	}
	var page struct {
		Items      []library.Library `json:"items"`
		NextCursor *string           `json:"next_cursor"`
	}
	if err = json.Unmarshal(r.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first page: %+v %v", page, err)
	}
	firstID := page.Items[0].ID
	if r = b.request(t, "GET", "/api/v1/libraries?limit=1&cursor="+*page.NextCursor, "", nil); r.Code != 200 {
		t.Fatalf("second page: %d", r.Code)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextCursor != nil || page.Items[0].ID == firstID {
		t.Fatalf("second page: %+v %v", page, err)
	}
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, `{"name":"missing fields"}`, map[string]string{"X-Library-Revision": "1"}); r.Code != 400 {
		t.Fatalf("validation: %d", r.Code)
	}
	if r = b.request(t, "DELETE", "/api/v1/libraries/"+item.ID, "", nil); r.Code != 204 {
		t.Fatalf("delete: %d", r.Code)
	}
	if r = b.request(t, "GET", "/api/v1/libraries/"+item.ID, "", nil); r.Code != 404 {
		t.Fatalf("deleted get: %d", r.Code)
	}
}
