# Project Architecture and Shared Conventions

## Authority and implementation status

This baseline was established on 2026-11-06 by `00-bootstrap-guidelines` and implemented for the application foundation by `initialize-projects`. The independent roots are `frontend/` (Vite/React) and `backend/` (Go module). The shared contract lives at repository-root `api/openapi.yaml`. Health checks, SQLite migration startup, development proxy, production embedding and quality tooling exist. The `admin-auth` task adds single-administrator initialization, cookie sessions/CSRF, initial settings and local password recovery; other V1 business modules remain future work. See the [backend authentication contract](../backend/authentication.md) and [frontend authentication contract](../frontend/authentication.md). Unlinked snippets in feature guidelines are illustrative rather than implemented business features.

The user's Notion checklists define architecture:

- [Backend V1.0](https://app.notion.com/p/401138389edc811fabfcefbf05f26845), last edited 2026-11-06 04:40 Asia/Shanghai.
- [Frontend V1.0](https://app.notion.com/p/401138389edc814fb54be80c599600ef), last edited 2026-11-06 04:42 Asia/Shanghai.

Architecture requirements below come from those pages. Directory names, API field conventions, formatter settings, and quality gates are project decisions established during this task. They are not claims about requirements stated in Notion. A later architecture change must update the relevant spec and API contract together; do not silently substitute stacks.

## Selected architecture

- Backend: Go, Gin, GORM, SQLite via `gorm.io/driver/sqlite` and `mattn/go-sqlite3`, goose versioned SQL migrations, `robfig/cron/v3`, persistent jobs with an in-process worker pool, SSE, `log/slog`, constructor injection.
- One modular monolith process runs HTTP, scheduling, and workers. Deploy one application instance with a local SQLite database. Build with CGO and a C compiler; use a compatible Debian runtime.
- Startup configuration uses environment variables/YAML; business settings live in the database. External credentials are encrypted with an independently configured master key. Admin authentication uses one administrator, password hashing, session cookies, and CSRF protection. Playback credentials are separate.
- Frontend: React 19+, Vite, TypeScript, HeroUI v3, Tailwind CSS v4, React Router, TanStack Query, limited Zustand, Lucide React. HeroUI's React Aria foundation supplies standard interaction behavior.
- Use CSS transitions first. Add Motion for React, TanStack Table, or TanStack Virtual only when a concrete feature needs them. Prefer HeroUI Table and its documented v3 virtualization mechanism when available for the pinned version; verify support before promising an API.
- Embed the frontend build using `go:embed`; serve SPA routes without swallowing API, health, playback, or static-asset errors. Vite development requests should use a same-origin proxy to the backend.
- Toolchains are pinned to Go 1.26.9, Node.js 24.13.0 and pnpm 11.28.5. `backend/go.mod`/`go.sum` and `frontend/pnpm-lock.yaml` own exact dependency versions. Keep compatible versions pinned and validate the complete embedded build when updating them.

## V1.0 boundaries and invariants

V1.0 includes authentication/settings, connections, cloud directory browsing, libraries, scanning/sync, STRM management, playback resolution, jobs, the media wall, metadata upload, and logs/audit. The media wall and metadata upload are in scope for V1.0, not optional later additions.

- SQLite: local disk, WAL, foreign keys, busy timeout, bounded write concurrency, short transactions. Network/filesystem work stays outside database transactions. Use a consistent SQLite backup, then reconcile file state after restore before re-enabling cleanup/overwrites.
- Jobs: persist runs, items, effects, schedules, and resource locks. Claim using conditional writes and verify affected rows. Define cancellation, bounded retries, idempotency, and restart verification; do not assume exactly-once external effects.
- Synchronization: record completeness per directory. Failed/incomplete scans never prove deletion. Lock by library output path and remote destination file; rate-limit 115 requests per connection.
- Files: track STRM ownership and content hashes; user-modified content becomes a conflict. Cleanup is disabled by default and requires preview plus quarantine outside media scan directories. Never overwrite or delete an unowned file merely because its name matches.
- Playback: STRM stores a stable application URL; resolve temporary 115 links at playback time. Cache only within link validity. Keep temporary links and playback credentials out of logs.
- Metadata: explicit local/remote mappings and sync baselines detect conflicts. Verify uncertain uploads before retrying.
- Container storage mounts: `/app/data` persistent database/state; `/app/media` default output/media sidecars (latest user-selected default in `admin-auth`); `/app/storage/quarantine` isolation; `/app/cache` rebuildable images; `/app/logs` rotation; `/app/backups` consistent backups. These are paths inside the Docker container; local development keeps its database under the project at `backend/data/mediahub.db`. Define shared media path mappings for Emby/Jellyfin.

## Shared HTTP contract decisions

The canonical contract lives at `api/openapi.yaml`. Generate Go transport types with the pinned `oapi-codegen` and TypeScript types with the pinned `openapi-typescript`; `make generate` updates both and `make check-generated` verifies repeatability. Never hand-edit generated output. Database models, domain models, and transport DTOs remain separate.

| Concern | Project convention |
| --- | --- |
| Routes | `/api/v1/*` management; `/api/v1/events` SSE; `/p/:asset_id` playback; `/health/live`, `/health/ready`; `/` SPA |
| JSON | `snake_case` fields; camelCase local TypeScript names; Go JSON tags define transport names |
| IDs | Opaque strings at the HTTP boundary, including numeric provider IDs; never coerce 115 IDs to JavaScript numbers |
| Time | RFC 3339 UTC strings for instants; render in the viewer's locale; duration fields include units such as `duration_ms` |
| Optionality | Declare omitted versus `null` explicitly in OpenAPI; collections are arrays, including empty `[]` |
| Lists | Paginate potentially large lists. Initial cursor convention: positive `limit` (default 50, maximum 200), opaque `cursor`, `{items, next_cursor}` with `next_cursor: null` at the end. Specify ordering and any provider-specific limits per operation. |
| Errors | Non-2xx response `{error: {code, message, request_id}}`; stable machine code, safe human message; field errors may be added only through OpenAPI |
| Requests | Validate/generate a bounded `X-Request-ID`, return it in responses and correlate logs; do not trust an unlimited raw header |
| Success | Use resource DTOs and meaningful HTTP status codes; no global success/error wrapper returning HTTP 200 for every outcome |

Use 400 for malformed/invalid input, 401 for missing/expired authentication, 403 for denied/CSRF failures, 404 for absent resources, 409 for state/conflict failures, 429 for throttling, and 500 for unexpected internal errors. Use 502/503/504 for upstream/unavailable/timeout cases where appropriate. Async work returns 202 with a persisted job reference. Specify endpoint-specific behavior in OpenAPI before implementing it.

## Authentication and event boundaries

- Cookies are HttpOnly and use an explicit SameSite policy; use Secure in HTTPS deployments. Validate CSRF on state-changing management requests, including login/session transitions where applicable. Keep tokens and passwords out of URLs and browser persistence; same-origin cookies authenticate SSE.
- Keep health checks minimal and non-sensitive. Restrict readiness details; health bypasses admin sessions but must not expose credentials or configuration.
- SSE is a notification channel for durable database state. Define event names, payload schemas, identifier semantics, and reconnect behavior in the contract. Payload fields follow the JSON/ID/time rules above.
- The initial convention uses connection-scoped event IDs, not durable replay. Reconnection triggers invalidation/refetch of subscribed queries; clients must not assume `Last-Event-ID` implies replay. If replay is added, persist events and document retention/cursor expiry first.
- Validate events at the frontend boundary, ignore unknown event kinds safely, and refresh affected query keys. Release subscriptions on logout/unmount, stop retrying unauthenticated streams, and bound buffers/slow-consumer handling. Never put side effects behind EventSource automatic retries.

## Readability and verification

Use clear English identifiers and English spec documentation. Comments explain intent, invariants, and tradeoffs; UI text follows the product's language. Prefer explicit control flow, small cohesive units, and constructor/prop injection. Extract shared code when callers share a stable concept, not merely because expressions look similar. Avoid speculative frameworks and catch-all `utils` modules.

Backend gates are in [backend quality](../backend/quality-guidelines.md); frontend gates are in [frontend quality](../frontend/quality-guidelines.md). Run `make check` for local gates, `make test-e2e` for browser checks, and `make build smoke` / `make docker-build docker-smoke` for production assembly. The [initialization contract](../backend/initialization.md) describes executable startup and transport boundaries. Documentation-only changes require link, consistency and whitespace checks; implemented application behavior requires its appropriate tests.
