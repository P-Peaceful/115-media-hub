# Backend Directory Structure

## Target layout

The [baseline](../guides/project-baseline.md) records provenance and the absence of application source. Use this target layout during initialization; create feature packages only when implementing them.

```text
cmd/mediahub/                  process entry point
internal/app/                  configuration, construction, startup/shutdown
internal/domain/<feature>/     business types and invariants
internal/service/<feature>/    use cases and consumer-owned repository interfaces
internal/repository/sqlite/    GORM models, mappings, queries, transactions
internal/http/                 Gin routes, middleware, handlers, DTO mapping
internal/http/generated/       oapi-codegen output
internal/adapter/              115, Emby/Jellyfin, filesystem boundaries
internal/jobs/                 scheduler, worker pool, retry/recovery
internal/web/                  embed declaration and copied frontend dist/
api/openapi.yaml               canonical HTTP and event schemas
migrations/                    goose SQL migrations
web/                           frontend source (see frontend specs)
```

Keep the embed declaration with the copied build output under `internal/web/`; `go:embed` cannot traverse `..`. Initialization must define the build-copy step; do not commit a fabricated production bundle.

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
