# Media library configuration contract

## 1. Scope / trigger

Library management persists configuration only. It does not scan the drive, run synchronization, generate files, calculate execution statistics, or imply that an enabled library is running. See `backend/internal/service/library/service.go` and the frontend media-library feature.

## 2. Signatures

`GET|POST /api/v1/libraries`, `GET|PUT|DELETE /api/v1/libraries/{id}`, and `PATCH /api/v1/libraries/{id}/enabled`; `api/openapi.yaml` is the canonical schema. Migration `00004_media_libraries.sql` stores configuration JSON, revision, and timestamps in `media_libraries`.

## 3. Contracts

All endpoints require the existing administrator session; mutations require CSRF proof. Lists use ascending opaque IDs and the standard limit/cursor envelope. Library configuration includes the 115 directory ID and display path, output path, media type, output options, structure, mappings and filename/link rules. IDs are strings. Directory display paths are saved snapshots, not proof that an account or directory still exists. Configuration survives restart and remains readable when 115 is unavailable.

Responses include a positive integer `revision`. PUT must send the revision originally loaded with the form in `X-Library-Revision`; the repository uses a conditional update to prevent a concurrent write after validation. A successful update or enable/disable changes revision. Re-fetch after a conflict. Deleting a library removes its configuration only; its successful destructive audit and deletion share a transaction, and an audit failure rolls the deletion back. Do not log paths or credentials. Service-owned business logs include operation, outcome, duration, request ID and available library ID; failures carry a stable error code.

## 4. Validation and errors

Malformed/missing fields or revision, invalid configuration, and invalid paging -> 400; missing session -> 401; invalid CSRF -> 403; missing library -> 404; stale revision or concurrent modification -> 409; storage/audit failure -> 503. Field values and bounds are specified in OpenAPI and validated by the service. A valid 115 directory ID is a string of 1–32 decimal digits; output path must be absolute.

## 5. Good / base / bad cases

- Good: load revision 1, edit and PUT with revision 1, then display the returned configuration and new revision.
- Base: display saved settings without live provider access; show unavailable execution statistics rather than inventing zeros.
- Bad: overwrite an edited library with a stale form, present enabled as actively running, or remove local/remote media when deleting the configuration.

## 6. Tests required

Exercise authenticated CRUD, CSRF, field validation, paging and configuration round trips; stale form and concurrent write conflicts; enable/disable and deletion; audit failure rollback and media-file preservation. Verify business outcomes, safe attributes and errors in logs. Frontend checks should cover directory selection/failure, editable configuration, invalid fields, unavailable operations and two-column/narrow layouts.

## 7. Wrong vs correct

Wrong: read the latest revision on the server and blindly save a stale browser form. Correct: require the form's original revision and atomically compare it with the stored version.
