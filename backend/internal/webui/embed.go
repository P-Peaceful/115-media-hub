//go:build !embedui

package webui

import "io/fs"

// Files returns nil in backend-only development. Use Vite for the UI.
func Files() fs.FS { return nil }
