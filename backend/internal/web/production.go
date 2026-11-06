//go:build production

package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var assets embed.FS

// Assets returns the actual frontend build copied before production compilation.
func Assets() (fs.FS, error) { return fs.Sub(assets, "dist") }
