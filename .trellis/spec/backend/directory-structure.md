# Backend Directory Structure

## Target layout

The [baseline](../guides/project-baseline.md) records architecture provenance and implementation status. The Go module lives in `backend/`; create feature packages only when implementing them.

```text
backend/go.mod                     independent Go module
backend/cmd/mediahub/              process entry point
backend/internal/app/              configuration, construction, startup/shutdown
backend/internal/domain/<feature>/ business types and invariants (when needed)
backend/internal/service/<feature>/ use cases and repository interfaces (when needed)
backend/internal/repository/sqlite/ GORM access, connections and transactions
backend/internal/http/             Gin routes, middleware and DTO mapping
backend/internal/http/generated/   oapi-codegen output
backend/internal/adapter/          external boundaries (when needed)
backend/internal/jobs/             scheduler and workers (when needed)
backend/internal/web/              embed declaration and copied frontend dist/
backend/migrations/                embedded goose SQL migrations
api/openapi.yaml                   shared canonical HTTP and event schemas
frontend/                          independent frontend project
```

Keep the embed declaration with the copied build output under `backend/internal/web/`; `go:embed` cannot traverse `..`. The root production build must copy the real frontend output before compiling with the production tag; do not commit a fabricated production bundle.

## Dependency rules

- HTTP translates transport data and calls services. `gin.Context` stays here; services/repositories/adapters/jobs use `context.Context`.
- Domain owns business concepts and has no Gin, GORM, or transport dependencies.
- Services orchestrate use cases, permissions, and transaction boundaries. Define narrow interfaces where consumed; repositories and adapters implement them implicitly. Concrete SQL mechanics remain in the repository.
- Jobs call services/adapters through explicit dependencies; handlers submit persistent jobs rather than spawning request-owned goroutines for durable work.
- `internal/app` wires concrete dependencies with constructors and owns lifecycle. Shutdown stops admission, cancels/drains workers within a deadline, and closes resources.
- Distinguish domain, persistence, and API types. Do not expose GORM models as JSON or import HTTP packages into services.

## Naming and organization

Use short lowercase Go package names without underscores; avoid `common`, `base`, and miscellaneous `utils`. Use `snake_case.go` filenames and `_test.go` test companions. Exported names use MixedCaps with standard initialisms (`HTTPClient`, `AssetID`); private names use mixedCaps. Constructors use `New...` and return initialization errors explicitly. Do not prefix interfaces with `I` or add an interface for every struct.

Keep code with its owning feature; promote it only when actual consumers require it. Avoid circular dependencies, global database/client variables, side effects in `init`, and empty directories standing in for implemented modules.

Reference: [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
