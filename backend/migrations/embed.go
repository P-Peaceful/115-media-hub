// Package migrations contains the versioned SQL schema migrations.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
