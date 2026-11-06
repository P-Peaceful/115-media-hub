//go:build !production

// Package web selects frontend assets for development or production builds.
package web

import "io/fs"

// Assets returns nil in development; Vite serves the frontend independently.
func Assets() (fs.FS, error) { return nil, nil }
