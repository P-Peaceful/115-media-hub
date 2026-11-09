# Cloud-drive browsing contract

## 1. Scope / trigger

Read-only 115 directory browsing crosses the authenticated SPA, management API and Cookie-based upstream. `api/openapi.yaml` owns the transport schema; the credential stays server-side. Search is intentionally unavailable, and media-library association is not yet implemented.

## 2. Signatures

`GET /api/v1/cloud-drive/files?directory_id=0&limit=50&sort=name&order=asc&type=all` accepts an optional opaque `cursor`. Implemented in `backend/internal/service/provider115/files.go` and `backend/internal/http/provider115.go`.

## 3. Contracts

Directory IDs are decimal strings of at most 32 digits. `limit` is 1–200. Sort is `name`, `size`, or `modified_at`; order is `asc` or `desc`; type is `all`, `video`, `audio`, `image`, or `document`. Cursor is bound to the directory, limit, sort, order and type. Response contains directory ID, path, items, and nullable `next_cursor`; file sizes are decimal strings or null and times are RFC 3339 or null. The 115 upstream uses offset pagination, which may shift as a directory changes; refreshing starts at page one. An empty upstream page before the reported count is reached is an error, not a complete listing. Account replacement/removal during a request invalidates the result.

For the observed 115 web endpoint, pass `cur=1` and `custom_order=1` along with `o` and `asc`: without `custom_order`, actual 115 responses ignored requested ordering. Validate response directory and path identity because a nonexistent ID may fall back to root. The returned path must start with root ID `0` and end with the requested directory ID; reject incomplete paths so copy never removes a real directory as if it were the root label. `te` is the observed modification time. Requests are bounded and must not log or return credentials or raw provider payloads. These are observed upstream behaviors, not a guaranteed public API.

## 4. Validation and errors

Invalid query/cursor → 400 `validation_failed`; missing administrator session → 401; missing directory → 404 `not_found`; unconfigured or explicitly invalid 115 authorization → 409 `provider_not_configured` or `provider_auth_invalid`; upstream/network or credential-race failure → 503. A network failure does not establish that authorization expired.

## 5. Good / base / bad

Good: page through a real directory with matching cursor and consistent parameters. Base: empty directory returns an empty array and null cursor. Bad: silently treat root fallback as the requested directory, report an incomplete page as the end, or use a cursor with another filter.

## 6. Tests required

Exercise paging and cursor validation, directory identity, unknown/invalid upstream responses, account replacement and credential redaction using a mocked upstream. Verify frontend directory history, refresh, error states and details. Report real-provider checks separately; small live samples do not establish all types or pagination behavior.

## 7. Wrong vs correct

Wrong: set `o=file_name` and assume the provider sorted accordingly, or search only loaded rows. Correct: send `custom_order=1`, verify provider behavior, and leave unsupported search explicitly unavailable until full-directory semantics are reliable.
