# Cloud-drive browsing contract

## 1. Scope / trigger

Read-only 115 directory browsing crosses the authenticated SPA, management API and Cookie-based upstream. `api/openapi.yaml` owns the transport schema; the credential stays server-side. Search is intentionally unavailable, and media-library association is not yet implemented.

## 2. Signatures

`GET /api/v1/cloud-drive/files?directory_id=0&limit=50&sort=name&order=asc&type=all` accepts either a one-based `page` or an optional opaque `cursor` (mutually exclusive). Implemented in `backend/internal/service/provider115/files.go` and `backend/internal/http/provider115.go`.

## 3. Contracts

Directory IDs are decimal strings of at most 32 digits. `limit` is 1–200. Sort is `name`, `size`, or `modified_at`; order is `asc` or `desc`; type is `all`, `video`, `audio`, `image`, or `document`. Cursor is bound to the directory, limit, sort, order and type. Page requests use `(page - 1) * limit`, reject unsafe values and incomplete non-final upstream pages. Response includes `page` (null for a legacy unaligned cursor), `page_size`, and `total_pages` (at least one); if the directory shrinks an out-of-range page can return an empty list, and the client offers valid pages. Response contains directory ID, path, items, nullable `next_cursor`, and `total`, the non-negative safe integer supplied by the provider for the request at response time; reject missing, null, invalid or unsafe counts. Provider filter-count semantics require live validation and totals can change between pages; file sizes are decimal strings or null and times are RFC 3339 or null. The 115 upstream uses offset pagination, which may shift as a directory changes; refreshing starts at page one. An empty upstream page before the reported count is reached is an error, not a complete listing. Account replacement/removal during a request invalidates the result.

For the observed 115 web endpoint, pass `cur=1` and `custom_order=1` along with `o` and `asc`: without `custom_order`, actual 115 responses ignored requested ordering. Validate response directory and path identity because a nonexistent ID may fall back to root. The returned path must start with root ID `0` and end with the requested directory ID; reject incomplete paths so copy never removes a real directory as if it were the root label. `te` is the observed modification time. Requests are bounded and must not log or return credentials or raw provider payloads. These are observed upstream behaviors, not a guaranteed public API.

## 4. Validation and errors

Invalid query/cursor → 400 `validation_failed`; missing administrator session → 401; missing directory → 404 `not_found`; unconfigured or explicitly invalid 115 authorization → 409 `provider_not_configured` or `provider_auth_invalid`; upstream/network or credential-race failure → 503. A network failure does not establish that authorization expired.

## 5. Good / base / bad

Good: page through a real directory with matching cursor and consistent parameters. Base: empty directory returns an empty array and null cursor. Bad: silently treat root fallback as the requested directory, report an incomplete page as the end, or use a cursor with another filter.

## 6. Tests required

Exercise paging and cursor validation, directory identity, unknown/invalid upstream responses, account replacement and credential redaction using a mocked upstream. Verify frontend directory history, refresh, error states and details. Report real-provider checks separately; small live samples do not establish all types or pagination behavior.

## 7. Wrong vs correct

Wrong: set `o=file_name` and assume the provider sorted accordingly, or search only loaded rows. Correct: send `custom_order=1`, verify provider behavior, and leave unsupported search explicitly unavailable until full-directory semantics are reliable.

## Cookie request control and internal scanning

`internal/adapter/provider115` owns bounded HTTP transport, safe typed errors, account admission, cooldown and retries; `service/provider115` keeps credential storage/state, input validation and projections. One constructed client is shared by account reads, public browsing, playback and background pages. QR polling/login uses the same transport without the file-account queue; a login exchange is never automatically repeated.

- `MEDIAHUB_115_REQUEST_INTERVAL` defaults to `1s`, accepts `1s`–`60s`, and starts after the previous response has been consumed and closed. One account has one in-flight API request. Foreground work can pass at most four times while background work waits; a scan releases admission between pages. Changing Cookie for the same UID does not reset cooldown. Response inspection identifies business throttling while admission is still held, before releasing the next waiter. A throttled response from an old credential is discarded for data adoption but still extends that stable account’s cooldown.
- Foreground reads have a 45-second operation budget and 15-second attempt budget; background reads use `max(45s, configured_interval + 15s)` so a valid 60-second interval can admit a following page. Playback has a 15-second total. Caller deadlines win. Read attempts are at most three, with 1/2-second transient backoff. HTTP 429 shares Retry-After cooldown (seconds/date; default 30 seconds), which cannot be shortened to fit a caller. Cancellation, credential races and exhausted preview budgets do not trigger fallback. Network body-read/close failures are temporary failures and must not trigger a fast-to-standard rescan. Known HTTP authorization/not-found status takes precedence over an unreadable body. No raw URL, body or Cookie belongs in returned diagnostics.
- Public `Files` remains 1–200. Internal `ScanFiles` supports 1150; only explicit batch incompatibility permits restarting that directory at 200. A complete directory is the adoption boundary for standard synchronization; a directory is limited to 10,000 page-call rounds and over-limit results are discarded as incomplete. Each page call can separately consume up to three HTTP attempts; this is not a 10,000-send cap.
- Recursive full synchronization can use `RapidScan`: a verified root/path anchor, identity-checked `/category/get` pickcode, recursive `/files?show_dir=0` pages of 1150, and `/app/chrome/downfolders` pages of 5000. Reject duplicate/conflicting IDs, changing counts, missing parents, cycles and changed final root/credential. The maximum full ancestor path is 256 components, including cached prefixes; caching cannot bypass the depth limit. Adopt only a complete snapshot. Unsupported or untrustworthy snapshots permit one standard fallback; authorization, throttling, cancellation and exhausted budgets do not. `MEDIAHUB_115_FAST_SCAN=false` selects the standard path. Nonrecursive libraries use standard enumeration. These are observed private APIs, so simulation success does not establish live-provider compatibility.
- Preview uses only standard pages with its own request counter consumed at the actual send boundary, including retry attempts. Keep response limits separate: ordinary 1 MiB, playback 64 KiB, large file pages 4 MiB, directory tables 8 MiB.
