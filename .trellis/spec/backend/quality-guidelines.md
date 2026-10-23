# Backend Quality Guidelines

## Current baseline

Use the root [Makefile](../../../Makefile), pinned [go.mod](../../../backend/go.mod) and [development notes](../../../docs/development.md). Read [project context](../project-context.md) before treating a Notion requirement as delivered behavior.

```sh
make check
# Focused backend verification from repository root:
cd backend && CGO_ENABLED=1 go test ./...
```

`make check` rejects gofmt differences, runs `go vet ./...`, frontend type/lint/peer checks, then both test suites. There is no configured Go third-party linter or coverage threshold. Do not claim race testing, business acceptance tests or full security verification are included.

## Review and examples

[database_test.go](../../../backend/internal/database/database_test.go) uses a real temporary SQLite file and repeated opening to assert PRAGMAs, migration version and pool size. [router_test.go](../../../backend/internal/http/router/router_test.go) uses `httptest` and `fstest.MapFS` to assert HTTP/SPA boundaries without an external server:

```go
r := New(func(context.Context) error { return nil }, files)
w := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, tc.path, nil)
req.Header.Set("Accept", "text/html")
r.ServeHTTP(w, req)
```

Prefer tests of observable boundaries and failure outcomes. Configuration tests use `t.TempDir` and `t.Setenv`; avoid writing test state into the repository or depending on user configuration. Existing config/DB/router tests do not cover all future business behaviors.

For startup, packaging or serving changes run `make smoke`: [smoke.py](../../../scripts/smoke.py) starts the built binary in temporary storage, checks health, embedded index/deep links, reserved routes/assets, no business tables, and SIGTERM exit. Production builds need `CGO_ENABLED=1` and `embedui`; [Dockerfile](../../../deploy/Dockerfile) supplies the runtime. `make build` prepares UI resources; do not commit generated build output.

## Required review decisions

- API changes start in [OpenAPI](../../../api/openapi.yaml), then `make generate`; review the Go interface and TS schema together.
- Keep Gin in HTTP, GORM in future repositories, external I/O outside transactions, and resource cleanup in composition. No repository/service implementation exists yet.
- Never use `AutoMigrate`, silent startup fallback after invalid config, raw secret logging, or HTML success for reserved API paths. Each masks a boundary failure.
- Use `context.Context` and error returns; avoid fatal process exit inside library packages. Current process exit belongs to `main`.

## Future acceptance work

The [functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8) require incomplete scans to preserve files, cleanup off by default with preview/quarantine, user-modified STRM preservation, stable playback URLs, cancellation/restart recovery, metadata conflict/unknown-upload verification, and safe backups/restoration. Test these when their features are implemented; the foundation suite cannot establish them. Quarantine must stay outside scan roots, and media API health alone cannot prove sidecar filesystem access.

Review each business task for identity/path containment, bounded retries and resource locks, uncertain outcomes, and destructive-effect verification. Do not replace missing implementation with a successful placeholder endpoint or declare large-index/client-playback acceptance from infrastructure tests.
