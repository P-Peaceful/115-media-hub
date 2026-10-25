# Single-Administrator Authentication Contract

## 1. Scope / Trigger

Use this contract for startup credentials, `/api/auth` endpoints, management routes, and the frontend session boundary. This application runs as one instance with one configured administrator. Sessions live only in the process and expire 24 hours after creation; there is no refresh token, account table, or session table. The [OpenAPI contract](../../../api/openapi.yaml) owns HTTP payloads.

## 2. Signatures

```go
func config.Load(path string) (config.Config, error)
func security.New(username, password string) *security.Service
func router.NewWithAuth(ping func(context.Context) error, files fs.FS, service *security.Service, cookieSecure bool) *gin.Engine
func router.RegisterManagementRoutes(r *gin.Engine, auth handler.Auth, register func(*gin.RouterGroup))
```

`app.Run` creates one `security.Service` for the process. Register future management APIs under `/api/admin` through `RegisterManagementRoutes`, which applies `Auth.Middleware()`; a frontend route guard alone cannot authorize HTTP requests. The health routes stay public. Playback authorization requires its own contract.

## 3. Contracts

| Input | Contract |
| --- | --- |
| YAML `admin_username`, `admin_password` | Required plaintext deployment credentials; neither has a default. Trimmed emptiness is invalid; the password's original bytes are used for comparison. |
| `MEDIA_HUB_ADMIN_USERNAME`, `MEDIA_HUB_ADMIN_PASSWORD` | Each environment variable overrides its YAML field when present, including an explicit empty value. Go does not load `.env` automatically. |
| YAML `cookie_secure` / `MEDIA_HUB_COOKIE_SECURE` | Optional boolean; environment overrides YAML. Default `false` supports HTTP loopback. Set `true` for HTTPS deployment; do not infer from forwarded headers. |
| `POST /api/auth/login` | JSON `username` and `password`; 200 returns `authenticated: true`, `csrfToken`, `expiresAt`, and an opaque `media_hub_session` Cookie. |
| `GET /api/auth/session` | 200 returns `authenticated: false` without a valid Cookie, or `authenticated: true`, `csrfToken`, `expiresAt` with one. |
| `POST /api/auth/logout` | Requires a valid Cookie plus `X-CSRF-Token`; 204 revokes that session and clears the Cookie. |

Session IDs and CSRF tokens are generated with `crypto/rand`. The Cookie uses `HttpOnly`, `SameSite=Lax`, `Path=/`, and an absolute-lifetime Max-Age. The CSRF token is returned in JSON and held in frontend runtime memory only. Auth responses use `Cache-Control: no-store`. Never log or persist credentials, full Cookies, session IDs, or CSRF tokens. Config changes take effect on restart, which clears all old sessions. The deployer must protect access to plaintext ENV/YAML credentials.

The frontend [AuthProvider](../../../frontend/src/features/auth/AuthProvider.tsx) queries the session before [AuthGuard](../../../frontend/src/features/auth/AuthGuard.tsx) mounts management content. The shared [request helper](../../../frontend/src/shared/api/client.ts) sends same-origin Cookies and attaches the in-memory CSRF token to mutating requests; 401 clears the token and announces expiry. Redirects after login pass through `safeReturnTo`, accepting a single leading `/` and rejecting external, `//`, and backslash forms. Preserve only nonsensitive route/query context.

## 4. Validation & Error Matrix

| Condition | Required outcome |
| --- | --- |
| Missing/blank admin field, including explicit empty ENV | `config.Load` fails before server startup; no unauthenticated fallback. |
| Invalid `MEDIA_HUB_COOKIE_SECURE` | Configuration error without echoing its value. |
| Malformed/oversized login body | 400 generic `invalid request`; request body capped at 8 KiB. |
| Wrong username or password | Same 401 `invalid username or password`; no session Cookie. |
| Missing, expired, or restart-invalidated session | `GET /api/auth/session` returns 200 `authenticated: false`; protected management requests return 401. |
| Valid session, missing or invalid CSRF on mutation/logout | 403 `invalid csrf token`; handler action does not run. |
| Valid session and CSRF on logout | 204; server record revoked and Cookie expired. |
| `/health/live` or `/health/ready` without login | Existing public probe behavior remains. |

Authentication errors expose neither which credential matched nor secret material. Do not return a successful SPA shell for a missing `/api` route.

## 5. Good/Base/Bad Cases

- Good: YAML provides both credentials, ENV overrides only the username, and the new pair logs in. A valid Cookie and CSRF token can log out.
- Base: no Cookie returns `authenticated: false`; the route guard displays login while retaining a safe internal destination.
- Bad: an empty `MEDIA_HUB_ADMIN_PASSWORD` must fail startup even when YAML has a password. A CSRF-less POST under `/api/admin` must not reach its handler.

## 6. Tests Required

For changes to this contract, assert configuration precedence and fail-closed empty/blank fields in [config_test.go](../../../backend/internal/config/config_test.go). Assert constant 24-hour expiry, logout, invalid credentials, and CSRF comparison in [security/auth_test.go](../../../backend/internal/security/auth_test.go). Use `httptest` in [router/auth_test.go](../../../backend/internal/http/router/auth_test.go) to check status and payload, Cookie flags, public health, blocked management mutations, and absence of secrets in logs. Check safe return routes, in-memory token behavior, and 401 handling in the frontend auth/API tests. Review both generated outputs after OpenAPI changes. For an end-to-end change, run the root quality and smoke commands described in [quality guidelines](quality-guidelines.md); report checks actually run.

## 7. Wrong vs Correct

Wrong: register a management endpoint directly on the public router, rely on a hidden button for security, or place the session ID in browser storage. Correct: register it through `RegisterManagementRoutes`, enforce Cookie and CSRF server-side, and keep the frontend token in runtime memory. This prevents a direct HTTP request or leaked browser storage from bypassing the management boundary.
