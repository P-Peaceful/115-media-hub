# Runtime Logs

## 1. Scope

The runtime log API serves sanitized operational events to administrators. Collection runs alongside the existing JSON output and optional file rotation. The application owns startup and shutdown order. The task service forwards individual execution summaries after their task records have been persisted.

- [Application wiring](../../../backend/internal/app/app.go)
- [Collector](../../../backend/internal/service/logs/collector.go)
- [Collection lifecycle](../../../backend/internal/service/logs/service.go)
- [SQLite operations](../../../backend/internal/repository/sqlite/runtime_logs.go)

## 2. Signatures

| Method and path | Request | Successful response |
| --- | --- | --- |
| `GET /api/v1/logs` | `range`, `level`, `module`, `job_id`, `q`, `limit`, `cursor` | `RuntimeLogPage` with `items` and nullable `next_cursor` |
| `GET /api/v1/logs/stream` | `level`, `module`, `job_id`, `q` | SSE events `log.created` with `RuntimeLogEntry` and `log.cleared` with `ClearRuntimeLogsResponse` |
| `GET /api/v1/logs/export` | `format=log\|jsonl` and history filters | Attachment with `X-Export-Truncated` |
| `POST /api/v1/logs/clear` | `X-CSRF-Token`, same-origin proof, and `{scope: "before_7d" \| "before_30d" \| "all"}` | `{deleted_count, scope}` |

The [OpenAPI contract](../../../api/openapi.yaml) owns public names, enums and response types. Migration [00009](../../../backend/migrations/00009_runtime_logs.sql) creates `runtime_logs` with a monotonic integer primary key and fixed-width UTC timestamps. API IDs are opaque strings.

## 3. Contracts

- History defaults to 24 hours and 50 rows, with a maximum of 200 rows per page. Sort by `occurred_at DESC, id DESC`. Keep both keys in the cursor. Search uses ASCII case folding and literal substrings of message, operation, error code, job ID and request ID. For example, `q=10%25` finds the literal text `10%`.
- Collect with a 2,048-entry queue and batch inserts. Filter health probes, routine access, log read access and internal collector diagnostics. Permit only approved scalar details and redact sensitive text before queueing. Apply the configured slog level and leave terminal/file output semantics in the original handler. Log task summaries carry the persisted job ID.
- Retain at most 30 days and 50,000 entries through scheduled bounded pruning. Each batch commits before publication. Limit live streams to 16 connections, each with a 128-event buffer. An overflow closes that stream so the consumer can refetch. Event IDs increment within a connection. Recheck the session before every outgoing event and 15-second heartbeat. Bound frame writes to five seconds.
- Export uses a short read transaction to establish the maximum row ID and determine whether the filter matches more than 10,000 rows. Read subsequent pages in separate batches of 500 so a slow download releases the SQLite connection between queries. Concurrent cleanup can reduce the remaining rows. An error after the response starts aborts the attachment. Log one terminal export outcome with request correlation.
- Stop live admission before HTTP shutdown. Stop the request and task producers before draining collection and releasing the database. Clear flushes queued records, commits scoped deletion and a `clear_runtime_logs` security audit in one transaction, and publishes a clear event after commit. The clear operation targets `runtime_logs`. Audit, job history, task logs and rotated output use their existing storage.

## 4. Validation and Errors

| Condition | Result |
| --- | --- |
| Missing or expired administrator session | HTTP 401 `authentication_required` |
| Missing CSRF proof or different origin for clear | HTTP 403 `csrf_invalid` |
| Invalid enum, cursor, pagination, format, or query length | HTTP 400 `validation_failed` |
| Storage or stream capacity unavailable before response | HTTP 503 `service_unavailable` |
| Required audit insert fails | Roll back the deletion and return HTTP 503 |
| Session expires or is revoked during streaming | End the stream before its next heartbeat or event |

## 5. Good, Base and Bad Cases

- Good — Query history, subscribe to matching committed entries, and reconcile delivery gaps by fetching history after reconnect.
- Base — An empty list returns `items: []` and `next_cursor: null`. An empty export returns a successful zero-row attachment.
- Bad — Publishing raw handler attributes risks exposing credentials. Publishing before insert produces IDs that history cannot reconcile.

## 6. Required Tests

Verify SQLite upgrade from version 8, tie-breaking by row ID, literal search parity, bounded pruning, and clear transaction rollback. Check original logging output, redaction, queue saturation, committed IDs, slow-subscriber disconnect and shutdown draining. Exercise authenticated list/export/stream requests, CSRF-protected clear, truncation, session revocation and graceful shutdown with a real HTTP connection.

## 7. Correct Publication Sequence

Publish inside the collector worker after `InsertRuntimeLogs` returns committed entries with IDs. Publish `log.cleared` after `ClearRuntimeLogs` commits. Publishing from the handler before these calls would let clients observe records or state they cannot retrieve.
