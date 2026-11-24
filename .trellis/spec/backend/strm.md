# STRM generation and 302 playback

## 1. Scope

This feature generates locally managed UTF-8 STRM files during full synchronization of a configured 115 library, persists ownership, and resolves one file at a time for playback. It shares the task-center run history and the media-library configuration snapshot.

## 2. Signatures

| Entry point | Contract |
| --- | --- |
| `GET /api/v1/libraries/{id}/full-sync-preview` | Admin session. Scans the saved 115 scope and checks local output. Returns the library revision, completion flag, scanned video and valid candidate counts, failed source entry count, up to 20 source/output path samples, and output readiness. |
| `POST /api/v1/tasks` | Admin session and CSRF. Submit `{type: "strm", library_id}` for an enabled library with STRM output enabled, 302 mode, and a saved playback address. |
| `GET /api/v1/strm` | Admin session. Supports library ID, name query, name or generation-time sort, direction, page and limit. Total and page use the same filtered database snapshot. |
| `GET /api/v1/strm/{id}` | Admin session. Returns the registered metadata and `present`, `missing`, `modified`, or `unavailable`. Content is available for a matching registered file. |
| `GET` and `HEAD /p/{id}/{token}/{name}` | Authorizes with the artifact-scoped credential. On success returns an uncached 302 to the temporary 115 URL. |
| `LibraryConfig.strm_direct_domain` | Shared playback address reachable by the player. A bare host or IP uses HTTPS. Explicit HTTP(S) origins may include a port. |
| `strm_artifacts` and `strm_write_intents` | Migration 00006 establishes unique output-path ownership, persistent per-artifact credentials and recovery intents. Migration 00007 removes hash fields, adds `playback_url` and `previous_content`, and marks existing intents as legacy. A downgrade requires both tables to be empty. |

See [OpenAPI](../../../api/openapi.yaml), [generation service](../../../backend/internal/service/strm/generate.go) and [migration](../../../backend/migrations/00007_strm_playback_content.sql).

## 3. Contracts

- Enumerate stable directory and file IDs with the existing 115 client. Honor directory exclusion and recursion settings. Report partial subdirectory failures with safe identifiers and restart at the beginning on retried runs. Bind each artifact to the source account ID, source file ID, pick code and library ID.
- Preview uses the same saved recursion, exclusion, account and pagination rules as generation. It checks effective access to the output directory or nearest existing parent with no test writes. It returns `ready`, `will_create`, `missing`, `not_directory`, `permission_denied`, or `unavailable`. After the preview deadline it returns the portion scanned and `scan_complete: false`. Candidate counts refer to source qualification. Existing-file policy and ownership are applied at execution.
- During execution, create missing output path components through rooted directory handles below the nearest existing writable ancestor. Recheck output readiness at execution and coordinate writes for overlapping roots. On Linux, new-file publication uses a hard link and overwrite uses an atomic exchange. Compare the displaced STRM bytes with the expected previous contents before removing the temporary file. An overwrite race can leave the displaced data in a hidden temporary file for recovery.
- Record the expected playback URL on new registrations and the previous bytes on an overwrite intent. Persist the intent, then write and sync the file outside the database transaction. Complete registration and remove the intent in a short transaction. On retry, the expected new content can be registered, and the expected previous content can be retried. Conflicting content is preserved and reported. Keep registry entries and file bytes when deleting a library. Its playback stops.
- Legacy registrations preserve their identities and credentials. With an empty expected URL, validate the local single-line playback URL against the canonical route and its registered artifact, credential, and source suffix. Exact origin and format comparison starts when a new URL is recorded. Recover an interrupted first write only if the file matches that identity. For a legacy intent with an existing owner, keep the registered metadata and let synchronization apply the saved existing-file policy. Exclude the expected URL and previous contents from logs.
- The local output name replaces the final source extension with `.strm`. The playback URL uses the saved library address from the run snapshot and a random persistent artifact credential. With `original_name`, its final segment is the full escaped source filename, including the original extension. With `file_id`, the final segment is the source file ID. Existing artifacts keep their URLs until generated again with the overwrite policy. Provider identity and pick code stay internal. The Cookie playback adapter requests a fresh signed URL using the player's User-Agent, validates the source file and trusted 115 destination, and checks whether authorization changed during resolution. See [provider adapter](../../../backend/internal/service/provider115/playback.go).
- Full-sync service logs include output readiness, scan start and result, and generation start and result. The `phase` field identifies the current step. Active scans and generation log progress at directory and file boundaries at most once every 10 seconds per phase. Correlate events by `job_id`, `library_id`, `attempt`, and the submission `request_id` when present. Emit up to three WARN samples for each stable `error_code` per run. The terminal record gives `failure_counts`, `suppressed_failure_logs`, elapsed time, and the generated, skipped, failed, and conflict counts. A completed run with mixed item results includes `partial_result: true`.
- Reconciliation records outcomes and conflicts. Successful provider page reads appear at DEBUG. The resolver and adapter diagnose failures using approved identifiers. Service logs exclude filenames, paths, cookies, playback tokens, pick codes, playback URLs, and signed upstream URLs.

## 4. Validation and errors

| Condition | Result |
| --- | --- |
| Invalid list filters or pagination | 400 `validation_failed` |
| Missing admin session or CSRF | 401 or 403 on the management boundary |
| STRM unavailable or submitted for a plan | 409 `task_type_unavailable` |
| Disabled or ineligible library, including a missing playback address | 409 `conflict` |
| Ineligible preview | 409 `full_sync_unavailable` |
| Configured 115 source directory missing | 404 `provider_directory_not_found` for preview |
| Existing path is a file, directory inaccessible, or parent unwritable | Preview reports the specific output status. Execution logs a localized output error and stops before writes. |
| Unknown artifact, invalid playback credential, deleted library or missing provider file | 404 `not_found` on playback |
| Missing or mismatched 115 authorization | 409 `provider_not_configured` or `provider_auth_invalid` |
| Registry or upstream resolution failure | 503 `service_unavailable` |

## 5. Representative cases

- Generation leaves an existing external file intact, and a repeat run deduplicates a previously registered output.
- Two different source IDs with names such as `sample.mkv` and `sample.mp4` resolve to the same `sample.strm` target. Keep the registered owner and report the other source as a conflict.
- An approved overwrite keeps the artifact ID and credential. A changed local file produces a conflict and its contents are retained.
- A direct playback request from a media player reaches the registered file through the 302 route without admin cookies.

## 6. Verification

Cover traversal, overlap, same-stem sources, filesystem and database interruption, concurrency, playback URL redaction, and management authentication. Exercise preview, task submission, filtered run history, registry reads, detail payload and redirected media contents together. Assert the generated URL uses the address saved with the test library. Migrate a version 00006 database and verify that registration, credentials, pending intents and existing playback URLs can still be read. See [backend workflow test](../../../backend/internal/http/strm_test.go) and [generation tests](../../../backend/internal/service/strm/generate_test.go).

## 7. Correct file and playback boundaries

Derive output paths from registry-controlled identifiers and repeat ownership checks at publication. Serve the stable artifact URL and request a short-lived upstream URL only during playback. Persist the output ownership and playback credential across process restarts.
