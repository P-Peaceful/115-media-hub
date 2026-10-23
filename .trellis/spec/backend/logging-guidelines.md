# Logging Guidelines

## Current logger and lifecycle

[logging.New](../../../backend/internal/logging/logging.go) defines `New(dir string) (*slog.Logger, io.Closer, error)`. It returns a JSON slog logger writing to stdout and rotated `app.log`. [main.go](../../../backend/cmd/server/main.go) sets it as the default and defers the closer. Do not discard the close handle or create a logger per request. [Project context](../project-context.md) distinguishes this operational logging from future audit facilities.

The directory is created with `0700` and a new log file with `0600`; existing permissions are not repaired. Rotation uses 20 MiB, 5 backups, 14 days and compression. This bounds log growth; stdout retention remains the deployment's responsibility. Default slog handling emits INFO and higher; there is no configured debug-level switch.

## Events and privacy

The existing [HTTP middleware](../../../backend/internal/http/router/router.go) records only method, matched route template, status and duration:

```go
slog.Info("http request", "method", c.Request.Method,
    "route", c.FullPath(), "status", c.Writer.Status(),
    "duration_ms", time.Since(start).Milliseconds())
```

Unmatched paths may have an empty matched route. Do not substitute the raw URL for it: query tokens and identifiers can leak. Recovery emits only `slog.Error("http handler panic")`, with no recovered value or stack. [app.Run](../../../backend/internal/app/app.go) logs server starting (address) and stopped. These are the available examples; there is no business logging helper, request ID middleware or audit schema.

Use INFO for normal lifecycle events and ERROR for failed operations, following existing call sites. WARN/DEBUG are not currently demonstrated; introduce them deliberately if a task needs actionable warning or diagnostic events. Keep structured key/value fields stable and useful; prefer safe internal IDs over entire objects.

## Future constraints and anti-patterns

The [backend architecture](https://app.notion.com/p/401138389edc811fabfcefbf05f26845) requires redaction. Never log Cookies, CSRF/admin/playback credentials, 115 tokens, raw request bodies, signed/temporary playback URLs, encryption keys or entire third-party responses. Future run/item IDs and safe error categories may aid diagnosis, but their persistence and audit policy need implementation first.

Wrong: `slog.Error("request failed", "url", c.Request.URL.String(), "error", err)`. Raw URLs and arbitrary external errors can contain secrets. Correct: the current route/status/duration event plus a reviewed safe category where needed. Do not duplicate logs at every propagation layer.

## Verification

`make check` exercises the repository baseline; no dedicated logging test exists. When changing middleware/rotation, add focused assertions for required fields, absence of secret sentinel strings, rotation settings and close behavior as applicable. Review both stdout and file sinks; do not treat the current generic panic log as a complete diagnostic or audit implementation.
