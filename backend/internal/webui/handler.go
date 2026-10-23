package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Handler serves assets and falls back to the SPA only for HTML navigation.
func Handler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if files == nil {
			http.Error(w, "Frontend is unavailable; use the Vite development server.", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err == nil && !info.IsDir() {
			if name == "index.html" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.FileServer(http.FS(files)).ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(files, "index.html")
		if err != nil {
			http.Error(w, "Frontend unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
}
