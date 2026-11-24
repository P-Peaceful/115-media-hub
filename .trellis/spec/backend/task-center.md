# Task center execution contract

## 1. Scope

The task center owns admission, scheduling, a bounded worker pool, job state, attempts, and history. Business modules own their processing and side effects. The application injects a type-to-module map into the job service. Production registers STRM generation as an on-demand module for media-library full sync. Eligibility requires an enabled media library with STRM output, 302 mode, and a saved playback address. See [application wiring](../../../backend/internal/app/app.go) and the [module interface](../../../backend/internal/service/jobs/service.go).

## 2. Signatures

The canonical schemas are in [OpenAPI](../../../api/openapi.yaml). Authenticated management routes are:

| Route | Behavior |
| --- | --- |
| `GET /api/v1/task-types` | List all seven types and the capabilities of currently registered modules. |
| `GET\|POST /api/v1/tasks` | List and count filtered history, or queue a task with `type` and `library_id`. |
| `GET /api/v1/tasks/{id}` | Read a task and its currently allowed actions. |
| `GET /api/v1/tasks/{id}/failures\|logs` | Read paginated failures or logs for the task. |
| `POST /api/v1/tasks/{id}/actions` | Pause, resume, cancel, retry the entire run, or retry its failed items. |
| `GET\|POST /api/v1/schedules`, `GET\|PUT\|DELETE /api/v1/schedules/{id}` | List and manage schedule configurations. PUT requires `X-Schedule-Revision`. |
| `PATCH /api/v1/schedules/{id}/enabled` | Update the enabled state. |
| `GET /api/v1/events` | Stream `task.changed` and `schedule.changed` invalidations. |

SQLite owns schedules, runs, items, and logs in [migration 00005](../../../backend/migrations/00005_task_center.sql). Task records retain library and plan identifiers when configuration is deleted.

## 3. Contracts

- The response type `TaskType` has `scan`, `strm`, `sync`, `check`, `cleanup`, `scrape`, `upload`. A registration requires a business `Run` callback. Declare pause and item retry only when the module can honor them. For manual-only modules, `can_schedule` is false and schedule admission rejects that type. Each attempt gets the current saved library revision as a configuration snapshot. Internal item keys are stable across resumed attempts. Module callbacks return administrator-safe summaries and honor context cancellation.
- The worker conditionally claims pending tasks and serializes active runs in a media library. Control signals and progress, item, and log writes are scoped to the active attempt. Modules coordinate external effects and checkpoints. A completed run with failed items and successful or skipped items has outcome `partial_failure`. To resume a running task after interruption, the module verifies its checkpoint or external effects before the service requeues it. Retry creates a linked run. Failed-item retry pages the original persisted failures.
- The scheduler evaluates each plan in its IANA timezone. The editor defaults to `Asia/Shanghai`. Custom rules use minute, hour, day-of-month, month and day-of-week fields. A null library selects all eligible enabled libraries. Occurrence submission and advancing the next time share a transaction. The plan ID, occurrence timestamp, and library ID form the deduplication key. Startup advances overdue plans past the missed time.
- Disabled plans have an empty next run time. Disabling or deleting a plan stops future submissions. Existing run histories remain available.
- Task lists use backward chronological ordering. Total uses all active filters. Type counts omit the type filter, and status counts omit the status filter. All page results use arrays and nullable next cursors.
- Mutations require CSRF. Accepted task commands return HTTP 202. SSE IDs are connection-scoped. Each frame has a changed resource ID and may include a terminal outcome when requested by the schedule. Execution callbacks coalesce updates and emit a trailing hint if the attempt stays active without further updates. Clients refetch persisted state after reconnect. Terminal outcome remains available in task history. A live toast depends on an authenticated connected client.

## 4. Validation and errors

| Condition | Response |
| --- | --- |
| Malformed fields, extra execution parameters, invalid schedule rule or missing edit revision | `400 validation_failed` |
| Missing session or invalid CSRF | `401 authentication_required` or `403 csrf_invalid` |
| Unknown run, plan or library | `404 not_found` |
| Known type without a registered module | `409 task_type_unavailable` |
| Invalid task transition or duplicate active retry | `409 task_state_conflict` |
| Stale plan revision or ineligible library | `409 conflict` |
| Storage failure or unavailable stream | `503 service_unavailable` |

## 5. Representative cases

- A registered module with a weekly plan creates a queued run at the occurrence and reports live progress using the task context.
- With an empty registry, capabilities show seven unavailable types and lists return real empty results. Task commands for unavailable types return a stable conflict response.
- A task whose upstream effect has an uncertain outcome requires module verification before the worker attempts recovery.

## 6. Verification

- Run backend quality gates and transport contract regeneration. Test concurrent claims and deduplication with SQLite, including upgrades from the previous schema and batch rollback.
- Exercise submission, schedule triggering, partial outcomes, cancellation, failed-item retry and restart recovery through injected test module callbacks.
- Check HTTP session and CSRF enforcement, unactionable types, plan edit conflicts, SSE notification frames, revocation, and graceful shutdown.

## 7. Responsibility boundary

Keep business commands in the owning service. Wire module registration at application construction and call it through the job service interface. The scheduler supplies identifiers and the saved library configuration. Network requests, paths, overwrite decisions, and other processing rules belong to the module.
