package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHealthAndSPARouting(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("<html>test shell</html>")}, "assets/app.js": {Data: []byte("export {}")}}
	r := New(func(context.Context) error { return nil }, files)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/health/live", 200}, {"/health/ready", 200}, {"/", 200}, {"/settings", 200},
		{"/assets/app.js", 200}, {"/assets/missing.js", 404}, {"/api/v1/missing", 404},
		{"/p/test", 404}, {"/health/unknown", 404}, {"/missing.css", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept", "text/html")
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("%s: %d, want %d", tc.path, w.Code, tc.status)
			}
		})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/settings", nil))
	if w.Code != 404 {
		t.Fatalf("POST SPA route: %d", w.Code)
	}
}
func TestReadinessFailsWhenDatabaseUnavailable(t *testing.T) {
	r := New(func(context.Context) error { return errors.New("private db error") }, nil)
	for _, tc := range []struct {
		path   string
		status int
	}{{"/health/live", 200}, {"/health/ready", 503}, {"/", 503}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
