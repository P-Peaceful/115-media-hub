# System Status Contract

## 1. Scope / Trigger

The dashboard displays process status across the authenticated management API and frontend. Keep this contract aligned with `api/openapi.yaml` and both generated clients.

## 2. Signatures

- `GET /api/v1/system/status` requires an administrator session.
- Success returns `SystemStatus`: `uptime_seconds` (non-negative integer) and `version` (non-empty string).
- `GET /health/ready` remains the independent public database readiness check.

## 3. Contracts

Uptime is elapsed time since the server process started, in whole seconds, and resets on process restart. The version is injected into `internal/http.Version` at build time; unversioned builds use `dev`. `MEDIAHUB_VERSION` configures `scripts/build.sh`, and the Docker build argument of the same name configures image builds. This is the running build identifier, not a claim that an update is available or installed. Management responses use `Cache-Control: no-store`. The dashboard validates the response at the API boundary and periodically refetches it; readiness and process information can fail independently.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Authenticated request | 200 with valid uptime and build version |
| Missing or expired session | 401 through the management boundary; no process details |
| Invalid or failed status response | Show unavailable data and allow retry; do not fabricate uptime or version |
| Database not ready | Public readiness returns 503; do not report it as ready from process status alone |
| No build version provided | Return `dev` |

## 5. Good / Base / Bad Cases

- Good: a signed-in user sees the actual process uptime and injected build version, while readiness is checked separately.
- Base: local development displays `dev` and starts uptime at process startup.
- Bad: expose build metadata in public health responses, infer uptime from browser load time, or label an unchecked version as latest.

## 6. Tests Required

Backend tests verify anonymous rejection, authenticated response, non-negative uptime and no-store behavior. Frontend tests cover valid and malformed responses, loading, independent failure and retry. Regenerate Go and TypeScript contracts and run the generated-file consistency check after OpenAPI changes. Exercise the actual build version injection when build wiring changes.

## 7. Wrong vs Correct

Wrong: add version and uptime to unauthenticated `/health/ready` and present the result as an update check.

Correct: serve process information through the authenticated status endpoint and use readiness solely for its existing health purpose.
