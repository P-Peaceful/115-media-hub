// Package migrations owns the embedded, versioned application schema migrations.
package migrations

import "embed"

// Files contains the SQL migrations shipped with the application.
//
//go:embed *.sql
var Files embed.FS
