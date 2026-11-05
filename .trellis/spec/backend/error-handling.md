# Error Handling

## Ownership

Services expose business errors independent of Gin/GORM. Repositories translate expected storage conditions (for example record absence or a uniqueness conflict) into service/domain errors. Adapters classify upstream failure, cancellation, and timeout. HTTP maps these errors using `errors.Is`/`errors.As`; do not compare error strings or leak database/SDK errors to clients.

Use stable semantic codes for expected failures; the exact codes belong in target `api/openapi.yaml`. HTTP status and envelope conventions are defined once in the [baseline](../guides/project-baseline.md#shared-http-contract-decisions).

## Propagation and logging

Wrap unexpected errors with useful operation context and `%w`; retain identity. Return errors rather than panic for expected failures. Never swallow an error or return a success-shaped zero value after a failed write. Log an unexpected failure once at its owning boundary with request/job correlation, then return a safe response. Avoid logging at every wrapping layer.

Illustrative service fragment, with a consumer-owned `repo` and business sentinel defined by the future feature:

```go
library, err := repo.FindByID(ctx, libraryID)
if err != nil {
    if errors.Is(err, ErrLibraryNotFound) {
        return Library{}, ErrLibraryNotFound
    }
    return Library{}, fmt.Errorf("load library: %w", err)
}
return library, nil
```

Do not include credentials or unredacted upstream URLs in wrapped messages. Handle cancellation separately from unexpected internal failure; a disconnected request is usually not an error-level incident.

## HTTP boundary

- Validate path/query/body data before service invocation and enforce body/collection size limits.
- Return the documented non-2xx status and `{error: {code, message, request_id}}` envelope. Unexpected messages are generic; diagnostic detail belongs in redacted logs.
- Use centralized mapping, including validation, auth/CSRF, not-found, conflicts, throttling, and upstream failures. Recovery middleware logs unexpected panics and emits a safe response when headers have not been sent.
- For SSE, validate authentication before opening the stream. After headers are committed, use the documented event/close behavior; do not attempt a JSON error response over an existing event stream.
- Never silently retry a write whose upstream result is unknown. Jobs persist the outcome and verify before deciding whether to retry.

## Verification

Table-test error identity through wrapping, HTTP mappings, safe messages, and request IDs. Verify malformed input cannot reach persistence, credentials never appear in responses/logs, and client cancellation terminates work that should be request-scoped. Durable jobs retain their own lifecycle after submission.
