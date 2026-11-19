# Media library configuration contract

## 1. Scope / trigger

Library management persists configuration only. It does not scan the drive, run synchronization, generate files, calculate execution statistics, or imply that an enabled library is running. See `backend/internal/service/library/service.go` and the frontend media-library feature.

Missing STRM statistics, recent synchronization, execution history, and operation previews use fixed frontend examples visibly labeled `演示数据`. The detail view displays real configuration and authenticated process uptime. The library enabled flag represents `已启用`. The output summary uses existing configuration fields.

The authenticated detail route has three navigable child pages at `/libraries/:id/{full-sync,full-sync/execution-preview,incremental-sync}`. The full-sync action leads to its preview, where users confirm execution. Preview cancel returns to full sync. The detail view displays every editable saved setting from the shared library query. Edit cancellation returns to the originating list or detail history entry, with the list as the fallback for direct entry. Demo actions update frontend state without mutation requests, background scheduling, or persistence. Incremental settings live in the current library's route group and reset on reload, library change, or exit. Keep example data out of real-request error handling.

## 2. Signatures

`GET|POST /api/v1/libraries`, `GET|PUT|DELETE /api/v1/libraries/{id}`, and `PATCH /api/v1/libraries/{id}/enabled`; `api/openapi.yaml` is the canonical schema. Migration `00004_media_libraries.sql` stores configuration JSON, revision, and timestamps in `media_libraries`.

## 3. Contracts

All endpoints require the existing administrator session; mutations require CSRF proof. Lists use ascending opaque IDs and the standard limit/cursor envelope. The create/update request contains `name`, `directory_id`, `directory_path`, `media_type`, `output_path`, `generate_strm`, `generate_nfo`, `generate_poster_cover`, `language_preference`, `scrape_source`, `exclude_directory_path`, `include_subdirectories`, `follow_directory_changes`, `file_encoding`, `link_type`, `strm_direct_domain`, `strm_link_format`, and `strm_existing_file_policy`. `enabled` is response-only and remains controlled by `PATCH /api/v1/libraries/{id}/enabled`.

Enum values are `media_type: movie | tv | anime | documentary | mixed`, `language_preference: zh-CN`, `scrape_source: tmdb | metatube | none`, `file_encoding: UTF-8`, `link_type: direct | proxy`, `strm_link_format: original_name | file_id`, and `strm_existing_file_policy: skip | overwrite`. The form defaults new libraries to `movie`, `zh-CN`, `tmdb`, `UTF-8`, `proxy`, `original_name`, and `skip` (existing-file policy). The optional excluded directory is a 115 drive display path (the same path representation returned by the directory picker), not a local absolute filesystem path. `strm_direct_domain` is empty or a hostname/IP with an optional port; it must not include a URL scheme or path. IDs are strings. Directory display paths are saved snapshots, not proof that an account or directory still exists. Configuration survives restart and remains readable when 115 is unavailable. Old configuration JSON is not migrated or guaranteed readable after the configuration contract changes.

Responses include a positive integer `revision`. PUT must send the revision originally loaded with the form in `X-Library-Revision`; the repository uses a conditional update to prevent a concurrent write after validation. A successful update or enable/disable changes revision. Re-fetch after a conflict. Deleting a library removes its configuration only; its successful destructive audit and deletion share a transaction, and an audit failure rolls the deletion back. Do not log paths or credentials. Service-owned business logs include operation, outcome, duration, request ID and available library ID; failures carry a stable error code.

Frontend edit submissions must project the loaded `Library` response into editable `LibraryInput` fields. Never serialize a response object directly: `id`, `created_at`, `updated_at`, and `enabled` are not writable, and `revision` belongs in the concurrency header. Cover successful edit with an assertion on the submitted keys; a mocked conflict response alone cannot detect this invalid-request regression.

The library-list deletion confirmation uses HeroUI `AlertDialog` with Header/Icon/Heading/Body/Footer and the `alertdialog` role. State explicitly that only configuration is removed, not local or cloud files. Focus Cancel initially and return focus on cancellation. Isolate deletion mutation feedback inside the dialog; while pending, disable Cancel/confirm and prevent Escape/backdrop dismissal and duplicate submissions. A failure retains the dialog for retry; a fresh open clears stale errors. Test these lifecycle cases and inspect long-name desktop/mobile screenshots using mocked deletion only.

The existing-file policy concerns an already-present STRM at the same target output path: `skip` preserves it, `overwrite` requests replacement when generation is implemented. The form labels it `STRM 已存在时` in output settings. Disabling STRM generation disables the policy control but retains and persists the selected value. This remains configuration only; do not add filesystem side effects.

Persisted-read exception: records saved before this policy was introduced may lack its JSON key. `sqlite.libraryFromModel` supplies `skip` only when `strm_existing_file_policy` is absent; explicit empty/null/unknown/wrong-type values are errors, not defaults. This read must not rewrite JSON, revision, or timestamps. New create/update requests still require the field. This narrowly scoped addition does not introduce conversion of other legacy configuration fields. SQL column defaults do not populate keys inside existing `config_json` values.

The STRM format row aligns its label and tabs and shows non-clickable `格式示例` below: `https://media.example.com/电影名称.mkv` for original name, `/123456789` for file ID. A supplied domain updates the example host; blank uses `media.example.com`. These are illustrative paths, not actual playback endpoints; direct/proxy do not imply different invented routes.

## 4. Validation and errors

| Condition | Result |
| --- | --- |
| Malformed body, missing required field, invalid enum/path/domain, missing or malformed revision, invalid paging | 400 |
| Missing administrator session | 401 |
| Invalid CSRF proof | 403 |
| Unknown library ID | 404 |
| Stale revision or concurrent modification | 409 |
| Storage or audit failure | 503 |

OpenAPI defines request field bounds and enums; the service enforces the same enum and domain constraints. A valid 115 directory ID is a string of 1–32 decimal digits; `output_path` must be an absolute local path. `exclude_directory_path` may be empty; when selected, accept the drive path as returned by the picker without requiring a leading slash. A direct domain may be empty; otherwise accept a hostname or IP literal with optional numeric port (0–65535), and reject schemes, user info, paths, out-of-range ports and an empty port after a trailing colon. The HTTP config decoder must reject all fields outside the writable contract, including response-only `enabled`.

## 5. Good / base / bad cases

- Good: create with all required fields and valid enum values; an empty optional excluded path/domain is accepted. Load revision 1, edit and PUT with revision 1, then display the returned configuration and new revision.
- Base: display saved settings without live provider access; detail-page mock execution statistics must carry the explicit demonstration label. Persist configuration preferences without starting scans, scraping, or file generation.
- Bad: send an unsupported language/source/link format, a URL where `strm_direct_domain` expects a host, a relative local output path, overwrite an edited library with a stale form, present enabled as actively running, or remove local/remote media when deleting the configuration.

## 6. Tests required

Exercise authenticated CRUD, CSRF, required fields/enums/domain validation, paging and round trips for every configuration field; include a non-absolute 115 display path for the optional excluded directory and reject a domain containing scheme/path. Cover stale form and concurrent write conflicts; enable/disable and deletion; audit failure rollback and media-file preservation. Verify business outcomes, safe attributes and errors in logs. Frontend checks should cover directory selection/failure, create defaults, editable configuration, field errors, removed controls, STRM format tabs and dynamic domain/format examples, default/overwrite round trips, disabled-but-retained existing-file policy, and responsive layouts. Verify child routes with direct entry, refresh, browser history and return links. Assert that demo interactions send no write requests.

When adding a required key inside persisted JSON, test a raw record saved before the key existed, not only upgraded fixtures. `backend/internal/repository/sqlite/library_policy_test.go` covers missing-policy list/detail reads, valid overwrite, explicit invalid values, and unchanged persisted metadata.

## 7. Wrong vs correct

Wrong: treat a selected 115 display path as a local absolute path, or accept a complete URL in `strm_direct_domain`. Correct: validate the excluded path as a bounded 115 display-path snapshot and accept only a host/IP with optional port for the direct domain.

Wrong: read the latest revision on the server and blindly save a stale browser form. Correct: require the form's original revision and atomically compare it with the stored version.
