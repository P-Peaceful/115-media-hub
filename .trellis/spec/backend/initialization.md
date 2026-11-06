# Application Initialization Contract

## 1. Scope / Trigger

This contract covers the runnable foundation introduced by `initialize-projects`: separate project roots, SQLite startup, health transport, development proxy and production resource assembly. Business functionality remains governed by the baseline and is not implied by the starter screen.

## 2. Signatures

- `GET /health/live` checks process liveness.
- `GET /health/ready` checks database availability.
- `api/openapi.yaml` owns shared schemas; generated output belongs in `backend/internal/http/generated/types.gen.go` and `frontend/src/lib/api/generated/schema.ts`.
- `backend/migrations/00001_application.sql` marks the SQLite file with `PRAGMA application_id = 115`; goose owns migration version tracking. There are no speculative business tables.
- The Go module is `github.com/wenzhengcheng/115-media-hub/backend`; run Go commands from `backend/`.

## 3. Contracts

Both healthy endpoints return HTTP 200 with `{"status":"ok"}`. Readiness failures return HTTP 503 with `{"error":{"code":"service_unavailable","message":"Service unavailable","request_id":"..."}}`. Responses carry a bounded `X-Request-ID`; accepted identifiers use ASCII letters, digits, dots, underscores and hyphens, up to 64 characters.

Startup environment variables are documented in `backend/.env.example`: `MEDIAHUB_HTTP_ADDR`, `MEDIAHUB_DATABASE_PATH`, `MEDIAHUB_LOG_LEVEL`, optional `MEDIAHUB_LOG_FILE`, and `MEDIAHUB_SHUTDOWN_TIMEOUT`. The shell or container loads environment variables; example files are not automatically read. Relative data paths resolve against process working directory.

Frontend development uses a same-origin Vite proxy. A production build copies `frontend/dist/` into `backend/internal/web/dist/` before compiling with `-tags production`. Default Go development builds do not require a frontend bundle. Copied assets are ignored build output; never modify them to change application behavior.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Valid startup and fresh local database | Apply embedded migrations before serving readiness |
| Repeated startup | Preserve database and migration state |
| Invalid address, logging level or shutdown timeout | Fail startup with actionable configuration error |
| Database fails readiness | Return safe 503 error envelope |
| Invalid or oversized request ID | Replace it with a generated bounded identifier |
| Unknown API, health, playback or asset path (including normalized dot/double-slash paths) | Return an error; never serve the SPA fallback |
| Missing production frontend bundle | Fail the production build rather than embed a fake page |

## 5. Good / Base / Bad Cases

- Good: install each project's dependencies from its own root, regenerate both transport types after changing OpenAPI, and run the full embedded build.
- Base: run the Go server and Vite separately; inspect the readiness status from the starter page.
- Bad: run Go commands at repository root, hand-edit generated transport files, or treat a missing JavaScript asset as a frontend route.

## 6. Tests Required

Backend tests must assert real SQLite WAL/foreign-key/timeout behavior, migration repeatability, health success/failure, safe error envelopes, request IDs and SPA exclusions. Frontend tests must cover loading, success, failure/retry and malformed transport data. Browser smoke tests must exercise the implemented starter flow. Integration checks must build real embedded assets and verify health, SPA routing and missing resources; container checks must run the CGO-compatible image.

## 7. Wrong vs Correct

Wrong: place `go.mod` at repository root and use `go:embed ../../frontend/dist`.

Correct: place `go.mod` in `backend/`, copy the actual frontend build into the owning `backend/internal/web/` package, and compile the production variant. This keeps project roots independent and respects Go embed containment rules.
