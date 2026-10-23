# Backend Directory Structure

## Current layout

The backend is a standalone Go module, composed explicitly rather than through a dependency container. [Project context](../project-context.md) separates implemented infrastructure from adopted business architecture.

```text
backend/
  cmd/server/                 flags, config, logger, signal context
  internal/app/               database and HTTP server lifecycle
  internal/config/            strict YAML + environment configuration
  internal/database/          SQLite opening and embedded migrations
  internal/logging/           slog + rotation
  internal/http/{handler,router,generated}/
  internal/webui/              development/production FS and SPA serving
  migrations/                 embedded versioned SQL
```

Real composition examples are [main.go](../../../backend/cmd/server/main.go) and [app.Run](../../../backend/internal/app/app.go). `Run(ctx context.Context, cfg config.Config) error` opens the DB, injects `sqlDB.PingContext` into `router.New`, starts HTTP and shuts down with the configured timeout. Keep resource ownership and cleanup at this boundary.

```go
// Existing composition in app.Run:
Handler: router.New(sqlDB.PingContext, webui.Files()),
```

## Placement and contract ownership

[Health](../../../backend/internal/http/handler/health.go) implements the generated `ServerInterface`; [router.New](../../../backend/internal/http/router/router.go) registers it through `generated.RegisterHandlers`. Add contract operations in [api/openapi.yaml](../../../api/openapi.yaml), run `make generate`, then implement the interface. Do not hand-edit generated files or create duplicate manual health routes.

Use lowercase Go package names and existing `*_test.go` placement alongside the implementation. SQL files use numbered goose migrations; see [database guidelines](database-guidelines.md). Shared API DTOs belong to `http/dto` only when needed; health currently uses generated types directly.

## Future boundaries

The adopted [backend architecture](https://app.notion.com/p/401138389edc811fabfcefbf05f26845) reserves `domain`, `service`, `repository/sqlite/model`, `adapter/{drive115,emby,jellyfin,filesystem}`, `jobs/{scheduler,executor,worker}`, `security`, `cache`, `event` and HTTP `middleware`/`sse`. These are placeholder directories, not reusable implementations.

When implemented, HTTP owns Gin contexts; services accept `context.Context`, orchestrate operations and own short transactions; repositories own GORM; adapters own external I/O. Keep transport DTOs, domain values and persistence models separate. Constructor injection belongs in composition. Persistent jobs, cron and SSE are future requirements, not running facilities.

## Wrong vs correct

Wrong: put drive requests or a long filesystem scan in a Gin handler or a DB transaction. This couples request lifetime and database locks to external work. Correct: keep the current handler boundary thin, and introduce a service/adapter under a separately scoped task when business behavior is required. Do not invent a service example as evidence: only the health infrastructure exists today.
