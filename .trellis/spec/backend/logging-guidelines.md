# Logging and Audit

## Runtime logging

Use injected `*slog.Logger` with a JSON handler. Configure logging centrally at startup, including output destinations, file rotation, retention, and bounded storage. `slog` does not rotate files by itself: `backend/internal/app/logging.go` uses pinned `lumberjack.v2 v2.2.1` when `MEDIAHUB_LOG_FILE` is set. It keeps a 10 MiB active file, three backups and seven-day retention, while preserving stdout output. Startup preflights file access; file-write failures emit a safe stable diagnostic to stdout. Avoid `fmt.Println`, the standard `log` package, and feature-specific logging libraries in application code.

Every backend business module must emit necessary operational logs; HTTP access logs alone do not satisfy this requirement. Cover key business actions, significant state transitions, and failures so an operation can be traced to its result.

Use stable snake_case attribute names. The standard handler emits `time`, `level`, and `msg`; correlate request events with `request_id` and job events with `job_id`. Business operation records require stable `module` and `operation` attributes. Completion records also include `outcome` (`succeeded`, `failed`, `canceled`, or `skipped`) and `duration_ms`; failures include a stable `error_code`. Include `library_id`, `connection_id`, `attempt`, and relevant resource IDs when applicable. Never fabricate absent IDs or repeat arbitrary payloads as attributes.

Illustrative usage after dependency construction:

```go
logger.With(slog.String("module", "jobs")).InfoContext(ctx, "job completed",
    slog.String("operation", "execute"),
    slog.String("job_id", jobID),
    slog.String("outcome", "succeeded"),
    slog.Int64("duration_ms", elapsed.Milliseconds()),
)
```

## Required business module coverage

This is an explicit user requirement established on 2026-11-06. For every implemented use case, identify its logging owner and required events during design/review. Services own business outcomes, jobs own execution lifecycle, and adapters own actionable upstream diagnostics. Pure domain functions do not need logger dependencies. Repositories normally propagate errors for the owning operation to log; logging at every layer creates duplicates rather than coverage.

| Business module | Minimum meaningful events |
| --- | --- |
| Authentication and system settings | Login/session outcomes, settings changes, backup/restore start/result, and post-restore reconciliation outcome; sensitive changes also require audit records |
| Connection management | Create/update/remove outcomes, authorization/refresh failures, connectivity check results, and connection availability changes |
| Cloud browsing | Directory listing failures/timeouts, provider throttling, and pagination diagnostics with connection/directory identifiers; routine successful listings may use DEBUG plus HTTP access logs |
| Library management | Library/rule/binding changes, validation conflicts, and affected library identifiers |
| Scanning and synchronization | Scan start/result, directory completeness failures, discovered/changed/skipped counts, and deletion decisions blocked by incomplete scans |
| STRM management | Generation/repair/preview/cleanup start/result and counts, ownership/hash conflicts, and quarantine/restore outcomes |
| Playback resolution | Resolution failures/timeouts, upstream throttling, and aggregate cache/redirect diagnostics; routine successes may be DEBUG or aggregated, never log temporary links |
| Job center | Submission, claim/skip, significant status transitions, retries with attempt/reason, cancellation, final outcome, and restart reconciliation |
| Media wall | Refresh/synchronization results, cache/upstream failures, and media-file mapping conflicts; aggregate routine image/cache traffic |
| Metadata upload | Mapping/baseline conflicts, batch results/counts, uncertain upload outcomes, verification decisions, and bounded retry outcomes |
| Logs and audit | Log export results, rotation/storage failures, stream subscription failures, and audit persistence failures; avoid recursively logging every log delivery |

New business modules must define equivalent coverage before delivery. Short read operations do not require INFO start/end pairs. Long-running or destructive operations require a start event and terminal outcome; partial results must carry failed/skipped counts and reasons rather than a misleading all-success summary. Unexpected interruption may leave a start without a terminal event, so recovery must log reconciliation and the recovered result. Rejected requests can be recorded at the HTTP/security boundary with correlation instead of creating duplicate service logs.

## Correlation and volume

Bind the stable module name through `logger.With` in construction, then attach operation/resource context at the owning boundary. Request-driven work includes `request_id`; accepted async work records both the submission request ID and persisted `job_id`, and execution events retain the job correlation. Parallel job items may add an item ID. Do not invent empty identifiers or require domain objects to know transport context.

Record one terminal business outcome at the owning boundary; HTTP access records and bounded adapter retry diagnostics may supplement it. Returned error wrapping must not produce identical error logs in the repository, service, job, and handler. Assign the terminal owner explicitly when a service can be called synchronously and by jobs.

Use counts and summaries for large scans, file batches, progress, playback, and media image traffic. Rate-limit/sample repeated diagnostic events with an explicit policy and suppressed-count summary. Never sample away required security/destructive audit records or the terminal result of a job. Health probes and successful routine reads must not flood INFO logs. Missing module coverage cannot be excused by a blanket sampling policy.

## Levels and boundaries

| Level | Usage |
| --- | --- |
| DEBUG | Bounded diagnostics disabled by default in production |
| INFO | Startup/shutdown, job transitions, meaningful completed operations |
| WARN | Degraded service, bounded retry, detected/recoverable conflict |
| ERROR | Failed operation requiring investigation; log once at its owner |

Expected validation/auth failures and normal cancellation are not automatically ERROR events. Do not log every progress tick or every scanned file at INFO. Bound log-follow buffers; slow SSE readers must not block workers.

## Redaction

Never log passwords, hashes, cookies, session/CSRF tokens, authorization headers, master keys, encrypted credential blobs, temporary playback links, or full upstream request/response bodies. Treat paths/file names as potentially sensitive: log IDs or a redacted summary when full paths are unnecessary. Allowlist attributes rather than logging whole structs; wrapping an upstream error can leak secrets too.

## Audit

Persist audit records separately from operational log files for authentication/security changes, credentials/settings changes, destructive actions, cleanup/restore, and other defined critical operations. Record actor identity, action, target identifiers, timestamp, outcome, and correlation ID without secret values. Services own auditing policy; do not infer a durable audit trail from an INFO message. Define handling of an audit persistence failure with the affected use case, especially before destructive operations.

## Verification

For each implemented business module, capture JSON output for its key workflow and assert module/operation, terminal outcome and duration, relevant request/job/resource correlation, and failure error code. Cover success, failure, and applicable cancellation/conflict/recovery paths. Verify one owning terminal record rather than duplicate errors at every layer. For high-volume reads, assert the documented DEBUG/aggregation behavior and that failures remain diagnosable. Check redaction and bounded diagnostic volume; critical audit records must remain complete. Test rotation/retention when logging is configured. Test that critical operations create the expected durable audit records without credentials.

Reference: [log/slog](https://pkg.go.dev/log/slog).
