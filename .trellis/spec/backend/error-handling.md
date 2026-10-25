# Error Handling and Infrastructure Contracts

## 1. Scope / Trigger

Use for startup configuration, lifecycle errors, health/API responses and SPA boundaries. [Project context](../project-context.md) is the authority boundary: there is no implemented business error taxonomy or universal success/error envelope.

## 2. Signatures

Real boundaries are [config.Load](../../../backend/internal/config/config.go), [app.Run](../../../backend/internal/app/app.go), [Health](../../../backend/internal/http/handler/health.go), and the constructors in [router](../../../backend/internal/http/router/router.go):

```go
func Load(path string) (Config, error)
func Run(ctx context.Context, cfg config.Config) error
func New(ping func(context.Context) error, files fs.FS) *gin.Engine
func NewWithAuth(ping func(context.Context) error, files fs.FS, service *security.Service, cookieSecure bool) *gin.Engine
// handler.Health has Ping func(context.Context) error
```

Wrap contextual failures with `%w` where the implementation does (for example `fmt.Errorf("HTTP server: %w", err)`); use `errors.Is` for `http.ErrServerClosed`. Some filesystem errors propagate directly. Do not claim all errors are wrapped or introduce string matching as a convention. [main.go](../../../backend/cmd/server/main.go) prints `startup failed:` to stderr and exits `1` on failure.

## 3. Contracts

Precedence is defaults < optional `-config` YAML < environment. Paths are relative to process working directory. Go does not load `.env` automatically. [Example YAML](../../../backend/configs/config.example.yaml) and [development notes](../../../docs/development.md) describe deployment configuration.

| Field / environment key | Default | Validation |
| --- | --- | --- |
| `http_addr` / `MEDIA_HUB_HTTP_ADDR` | `127.0.0.1:8080` | host:port with numeric port 1–65535 |
| `database_path` / `MEDIA_HUB_DATABASE_PATH` | `./data/115-media-hub.db` | nonblank |
| `log_dir` / `MEDIA_HUB_LOG_DIR` | `./logs` | nonblank |
| `shutdown_timeout` / `MEDIA_HUB_SHUTDOWN_TIMEOUT` | `10s` | parseable positive duration |
| `admin_username` / `MEDIA_HUB_ADMIN_USERNAME` | none | required; trimmed value nonblank |
| `admin_password` / `MEDIA_HUB_ADMIN_PASSWORD` | none | required; trimmed value nonblank; original bytes retained |
| `cookie_secure` / `MEDIA_HUB_COOKIE_SECURE` | `false` | parseable boolean |

Environment presence overrides YAML even with an empty value; empty values are not ignored. `KnownFields(true)` rejects unknown YAML fields and a second document is rejected. A returned config accompanying an error is not usable startup configuration. See [administrator authentication](auth-contract.md) for HTTP/Cookie/CSRF behavior.

## 4. Validation & Error Matrix

| Trigger | Current result |
| --- | --- |
| Missing config file / bad YAML | `open config: ...` / `decode config: ...` |
| Additional YAML document | `config must contain exactly one YAML document` |
| Invalid env duration | `invalid MEDIA_HUB_SHUTDOWN_TIMEOUT` |
| Blank admin credential or DB/log path, invalid address or nonpositive timeout | validation error; startup stops |
| Invalid `MEDIA_HUB_COOKIE_SECURE` | validation error; startup stops |
| `GET /health/live` | 200 JSON `{"status":"ok"}` independent of DB |
| `GET /health/ready`, DB ping succeeds | 200 JSON `{"status":"ok"}` |
| Readiness ping fails (2s context timeout) | 503 JSON `{"status":"unavailable"}`; no DB error detail |
| Recovered handler panic | 500 JSON `{"error":"internal server error"}`; generic log |
| Unmatched `/api`, `/p`, `/health` or descendants | 404 JSON `{"error":"not found"}` |
| Unmatched non-GET/HEAD route | 404 JSON; never SPA fallback |

[OpenAPI](../../../api/openapi.yaml) specifies health and administrator authentication responses. Generic router errors are current middleware behavior, not a generated domain error contract.

[webui.Handler](../../../backend/internal/webui/handler.go) serves existing files first. A missing extensionless GET/HEAD route falls back only when `Accept` contains `text/html`; missing `assets/` files and paths with extensions return plain HTTP 404. With no embedded FS, UI requests return plain 503. The SPA shell uses `Cache-Control: no-cache`; fallback HEAD has no body. Keep these distinctions when adding routes; a JSON client must not receive an HTML shell for a missing API.

## 5. Good/Base/Bad Cases

Good: valid YAML sets port 9090 and an environment override sets 9191; the latter wins. Base: defaults start local development. Bad: unknown YAML key or `MEDIA_HUB_DATABASE_PATH=''` must abort startup. For HTTP, good is a healthy readiness response, base is liveness with DB unavailable, bad is exposing the private ping error or converting `/api/v1/missing` into HTML success.

## 6. Tests Required

[config_test.go](../../../backend/internal/config/config_test.go) asserts precedence and rejects unknown fields, negative timeout, invalid address and blank DB path. [router_test.go](../../../backend/internal/http/router/router_test.go) asserts health/readiness statuses, deep links, reserved paths, missing assets and POST rejection. For changes, assert both status and public payload/privacy; add cases for newly affected validation and HEAD/Accept behavior. Do not imply these extra assertion points already exist. Run `make check` and `make smoke` for HTTP/startup changes.

## 7. Wrong vs Correct

Wrong: return `err.Error()` from failed readiness or turn all errors into a made-up envelope. Correct: preserve the generated `Health` status and keep internal details private. Future business error categories must be designed against the [backend architecture](https://app.notion.com/p/401138389edc811fabfcefbf05f26845), then added to OpenAPI and both consumers together. Authentication errors follow [their own contract](auth-contract.md).
