# Administrator Authentication Contract

## 1. Scope / trigger

Authentication/settings, first-run initialization and local password recovery span SQL, service, HTTP, CLI and frontend boundaries. Extend this contract together with `api/openapi.yaml` whenever any of those boundaries changes. The application has exactly one administrator; 115/provider authorization remains a separate feature.

## 2. Signatures

Canonical HTTP operations:

- `GET /api/v1/auth/bootstrap`
- `POST /api/v1/auth/initialize`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`

Maintenance command: `mediahub admin reset-password [--password-stdin]`. It operates on the configured existing administrator/database, not a public HTTP recovery endpoint.

Migration `backend/migrations/00002_administrator_auth.sql` owns `administrators`, `system_settings`, `admin_sessions` and `audit_records`. The administrator/settings primary-key check admits only singleton 1. Session tokens and CSRF proofs are hashed at rest. Credential migration downgrade is explicitly rejected; restore a consistent backup with the matching binary.

Source owners: `backend/internal/service/auth/` for policy, `backend/internal/repository/sqlite/auth.go` for SQL transactions, `backend/internal/http/` for transport/cookies and `backend/cmd/mediahub/` for maintenance input.

## 3. Contracts

Bootstrap has `initialized`, `authenticated`, `administrator`, `expires_at`, `csrf_token`; anonymous administrator/expiry are explicitly `null`. Session success has only public `administrator: {id, username}`, RFC3339 UTC `expires_at` and in-memory `csrf_token`. Raw session cookies never appear in JSON. Auth responses are `Cache-Control: no-store`.

Initialize accepts `username`, `password`, `service_url`, `output_directory`, `session_lifetime_days` and returns 201 with session. Login accepts `username`, `password`, `remember_login` and returns 200 with session. Logout returns 204. Unsafe requests require `X-CSRF-Token` bound to the current HttpOnly anonymous/session cookie plus origin validation. Rotate session/CSRF after initialization/login.

Cookies use explicit SameSite=Lax and Path=/; session cookies are HttpOnly. `MEDIAHUB_COOKIE_SECURE` is a startup boolean, default false for HTTP development; enable behind HTTPS termination. Do not trust arbitrary forwarded headers to choose secure behavior or rate-limit peers.

Default output directory is `/app/media`, explicitly chosen by the user. It is an absolute server/container path; storing it neither selects a client folder nor proves write access. Service URL is an HTTP(S) origin with no userinfo, path prefix, query or fragment. Session lifetime is fixed at 1/7/30 days (default 7), even when the browser cookie is not persistent.

Successful initialization/login/logout/reset audit participates in the same transaction as its mutation. Audit-write failure rolls back that mutation. Failed-login audit errors must not result in an issued session or a misleading credential-only result. Record redacted business outcomes with module/operation/outcome/duration/correlation; never record credentials or token values.

## 4. Validation and error matrix

| Condition | Contract |
| --- | --- |
| Bad JSON/body limit/unknown properties/invalid fields | 400, `validation_failed`, optional safe field errors |
| Incorrect valid-shaped credentials | 401, `invalid_credentials`, same message for username/password |
| Missing/expired/revoked session | 401, `authentication_required` |
| Missing/invalid CSRF or rejected Origin | 403, `csrf_invalid` |
| Login before setup | 409, `setup_required` |
| Repeat/concurrent losing setup | 409, `already_initialized` |
| Bounded admission/attempt limit | 429, `rate_limited`, `Retry-After` |
| Storage/mandatory-audit failure | Safe unavailable response; no partial mutation |

Usernames: 3–32 ASCII letters/digits/underscore/period/hyphen, case-sensitive. Passwords: 8–32 printable non-space ASCII characters (U+0021–U+007E), without trimming. Reject Chinese/non-ASCII, whitespace and control characters; do not add a mixed-character-class requirement. Password-confirmation is a UI responsibility, not a second persisted secret. Tokens are generated from explicit cryptographic random bytes with a defined encoding/length; `rand.Text` does not promise a forever-fixed length.

## 5. Good/base/bad cases

- Good: bootstrap cookie + matching proof → initialization → rotated session → authenticated bootstrap after process restart.
- Base: initialized anonymous user → login → browser-session cookie when remember=false → fixed server expiry still applies.
- Bad: two setup requests both save state, audit fails but credentials persist, a stale old-password login creates a session after password reset, or an anonymous response exposes settings. Transactions, singleton constraints and current-credential checks must prevent these cases.

## 6. Tests required

Use temporary-file SQLite for fresh/upgrade schema, singleton competition, audit rollback, session expiry/restart and reset preserving settings. Use controlled synchronization to test reset against an in-flight login; deleting existing sessions alone does not block a concurrent login that compared the old hash earlier. Service tests control time/cost and verify log redaction. HTTP tests exercise real issued cookies through CSRF/auth, origin rejection, cookie flags, body limits and preserved unknown-route JSON 404. CLI tests cover hidden/explicit-stdin input, absent administrator, invalid password and settings/session preservation.

Production smoke must initialize through actual HTTP with a cookie jar, then keep that cookie jar across binary/container restart to prove session persistence. Mocked browser tests do not establish this behavior.

## 7. Wrong vs correct

Wrong: assume `rand.Text()` returns an arbitrary hardcoded length, compare passwords outside a transaction and insert a session without checking whether credentials changed, or return 200 after an audit failure.

Correct: use an explicit cryptographic token format and test issuance → validation, condition session creation on the credential state that was verified, and commit security state plus its required audit atomically.
