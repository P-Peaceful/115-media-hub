package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

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
	body := `{"name":"电影","description":"","directory_id":"123","directory_path":"/电影","media_type":"movie","enabled":true,"output_path":"/media","generate_strm":true,"scrape_metadata":true,"generate_nfo":true,"preserve_structure":true,"structure_mode":"full","path_mappings":[],"link_type":"direct","link_expiry":"default","filename_rule":"original","strm_extension":".strm","nfo_name":"same_as_video","min_file_size_mb":100}`
	if r := b.request(t, "POST", "/api/v1/libraries", body, map[string]string{"X-CSRF-Token": "invalid"}); r.Code != 403 {
		t.Fatalf("csrf: %d", r.Code)
	}
	r := b.request(t, "POST", "/api/v1/libraries", body, nil)
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	var item library.Library
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil {
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
	updatedBody := strings.Replace(body, `"enabled":true`, `"enabled":false`, 1)
	updatedBody = strings.Replace(updatedBody, `"description":""`, `"description":"updated"`, 1)
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, updatedBody, nil); r.Code != 400 {
		t.Fatalf("missing revision: %d", r.Code)
	}
	if r = b.request(t, "PUT", "/api/v1/libraries/"+item.ID, updatedBody, map[string]string{"X-Library-Revision": "2"}); r.Code != 200 {
		t.Fatalf("update: %d %s", r.Code, r.Body)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.Revision != 3 || item.Description != "updated" || item.Enabled {
		t.Fatalf("updated: %+v %v", item, err)
	}
	if r = b.request(t, "GET", "/api/v1/libraries/"+item.ID, "", nil); r.Code != 200 {
		t.Fatalf("get updated: %d", r.Code)
	}
	if err = json.Unmarshal(r.Body.Bytes(), &item); err != nil || item.Revision != 3 || item.Description != "updated" || item.Enabled {
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
