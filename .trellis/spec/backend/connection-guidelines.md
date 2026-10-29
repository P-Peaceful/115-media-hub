# 115 Connection Contract

Use this contract when changing external-drive connections, QR authorization, or consumers of saved 115 credentials. The implementation lives in `internal/adapter/drive115`, `internal/service/connections`, `internal/repository/sqlite`, and the generated management API.

## Scenario: 115 QR connection lifecycle

### 1. Scope / Trigger

- Trigger: External credential persistence, QR sessions, connection APIs, or a new service consuming a saved drive connection.
- One active 115 connection is supported. Reauthorization keeps its connection ID and may replace its credential only after the remote account identity matches.
- The QR session is short lived and process local. Refreshing the management page requires starting a new session.

### 2. Signatures

- Adapter boundary: `StartQRCode(ctx, device)`, `PollQRCode(ctx, challenge)`, `CompleteQRCode(ctx, challenge, device)`, and `VerifyCredential(ctx, credential)` in `internal/adapter/drive115`.
- Management API: `GET /api/admin/connections/115`, `POST /api/admin/connections/115/qr-sessions`, `GET /api/admin/connections/115/qr-sessions/{sessionId}`, `GET /api/admin/connections/115/qr-sessions/{sessionId}/qr`, `POST /api/admin/connections/115/qr-sessions/{sessionId}/complete`, `POST /api/admin/connections/115/test`, and `POST /api/admin/connections/115/profile/refresh`.
- Persistence: `connections.credential_json` stores the verified credential JSON as plaintext in the server-side database; the partial unique index permits one active row with kind `115`.

### 3. Contracts

- QR start request is `{ "device": "web" | "alipaymini" | "wechatmini" }`; default UI selection is `web`.
- QR JSON responses contain only `sessionId`, `device`, `expiresAt`, and normalized `status`. The image endpoint returns `image/png`; upstream signatures and challenge parameters stay in backend memory.
- Connection summaries contain `configured` and, when present, `id`, `name`, `externalAccountId`, `status`, `capabilities`, and `updatedAt`. They never contain credentials.
- Connection summaries contain selected safe upstream profile values: account name, avatar, optional membership name/expiry, storage totals in bytes and retrieval time. Membership display is the upstream name verbatim; do not publish numeric `membershipLevel`, derive labels from `vip`, or repair a display name from a legacy numeric snapshot. Unknown fields stay absent/unknown; they must not be inferred as zero or ordinary membership.
- The coarse `membershipStatus` remains for compatibility and may reflect explicit upstream privilege signals. It is not a display-name source; numeric levels and text matching must not invent membership evidence.
- Persist a versioned (`schema_version`) profile snapshot in `profile_json`, separately from credential JSON. Profile enrichment is best effort and cannot invalidate a successfully verified connection. The protected `POST /api/admin/connections/115/profile/refresh` endpoint updates only this snapshot.
- QR status values are `creating`, `waiting`, `scanned`, `allowed`, `expired`, `canceled`, and `failed`. Connection status values are `unchecked`, `available`, `invalid`, `limited`, `unreachable`, and `unknown`.
- Credentials are intentionally stored unencrypted per the user decision. Protect the database file, host, and backups as material containing usable 115 login credentials.
- Management routes require the administrator session; mutations also require the existing CSRF header.
- 115 upstream failures may expose a sanitized human-readable message and upstream code to the authenticated administrator. Extract message/code fields only; never forward raw response bodies or headers. Redact cookies, tokens, QR signatures, and challenge parameters. QR polling carries the same safe message in its normalized session response.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Unsupported device | `400 invalid_request`; do not call 115 |
| Stored credential JSON malformed | Return a generic unavailable error; do not expose the stored value |
| Missing/expired in-memory QR session | `404 not_found`; no connection write |
| Completion before `allowed` | `422 invalid_request`; no connection write |
| New account differs from existing account | `409 conflict`; preserve old ID, credential JSON, status, and capabilities |
| Explicit invalid credential | `invalid`; never expose the upstream response |
| Network/timeout failure | `unreachable` or `unknown`; do not mark the credential invalid |
| Root read rejected after identity verification | `limited`; preserve account identity evidence and mark root read unavailable |
| 115 upstream error | Stable local category plus extracted, sanitized upstream message/code; no raw response body or headers |
| Unexpected database error | Generic error response; no SQL, credential, QR parameters, or upstream body |

### 5. Good/Base/Bad Cases

- Good: verify account identity and root read before persistence; compare account IDs; atomically persist plaintext credential JSON; return only a safe summary.
- Base: first successful authorization creates one connection whose stable ID is independent of the upstream QR challenge and whose capabilities record the verification evidence.
- Bad: returning the QR signature in JSON, returning cookies through an API or logging them, treating a timeout as invalid credentials, overwriting a connection after account mismatch.

### 6. Tests Required

- Adapter tests: normalize every upstream QR state, reject unknown devices, classify invalid credentials separately from transport failures, and assert upstream bodies are not returned.
- Service tests: first connection, same-account reauthorization preserving ID, different-account conflict preserving the old row, repeated completion idempotency, expiry/cancellation, capacity bounds, credential JSON parsing and network versus invalid status persistence.
- Persistence tests: goose migration and partial unique index, credential JSON persistence and transaction rollback.
- HTTP tests: admin/CSRF gates, QR image content type, safe error bodies, no credential/signature fields, and read-only connection test behavior.
- Frontend tests: device selection and Web warning, in-memory session reset on refresh, one completion per session, polling stop conditions, and no credential data in URL/storage.

### 7. Wrong vs Correct

#### Wrong

```go
if verifyErr != nil {
	connection.Status = "invalid" // A timeout does not prove the credential is invalid.
}
```

#### Correct

```go
switch {
case errors.Is(verifyErr, drive115.ErrCredentialInvalid):
	result.Status = "invalid"
case errors.Is(verifyErr, drive115.ErrUnreachable):
	result.Status = "unreachable"
default:
	result.Status = "unknown"
}
```

Keep upstream protocol types and error strings inside the adapter. The project-owned client replaces `SheltonZhu/115driver`; do not reintroduce its types in production or tests. Add public API changes to `api/openapi.yaml`, then regenerate both Go and TypeScript contracts.

For the membership name, use Cookie `GET https://webapi.115.com/user/vip_limit?feature=2` and its `data.user_limit.level_name`, documented by [p115client](https://p115client.readthedocs.io/en/latest/reference/module/client.html#p115client.client.P115Client.user_vip_limit) with [primary client usage](https://greasyfork.org/en/scripts/531685-%E4%B8%80%E9%94%AE%E5%A4%8D%E5%88%B6%E7%A3%81%E5%8A%9B%E9%93%BE%E5%92%8C%E6%8E%A8%E9%80%81%E5%88%B0115%E7%A6%BB%E7%BA%BF/code). The nav response's numeric `vip` is not a name. Bearer `vip_info.level_name` is a different protocol; do not introduce Bearer auth for this lookup. Expose the selected text as optional `membershipName`, store it in the safe profile snapshot and show it in both the connection page and sidebar; missing/invalid text or failure renders `会员信息未知`. Optional lookup failure must not invalidate verified identity or credentials. Names are text, never HTML.

## Scenario: Project-owned 115 Cookie client

### 1. Scope / Trigger

Use when extending `backend/internal/adapter/drive115`. Protocol behavior is informed by [115-station](https://github.com/pancras-loe/115-station), with local context, privacy and error contracts taking precedence over its handler implementation.

### 2. Signatures

Preserve `Adapter`, `BrowserAdapter`, `Credential`, `Directory`, `DirectoryPage` and `AccountProfile` as the service boundary. `StatDirectory(ctx, credential, directoryID)` resolves ancestors; `ListDirectory(ctx, credential, directoryID, offset, limit, kind, query)` returns one page. Raw QR challenge fields stay private to the adapter.

### 3. Contracts

- `/files` supplies `path` from root through the requested directory. Skip the upstream root label and render the local `我的网盘`; do not append the current directory twice.
- Directory IDs (`cid`/`pid`) and file IDs (`fid`) accept JSON integer or decimal-string representations, preserve exact decimal digits and become Go strings. Never decode IDs through `float64` or native `int`. Values above 2^53 must survive exactly.
- HTTP calls use request contexts, finite timeouts, a reused connection pool and per-request Cookie headers. Never retain a caller's context or mutable credentials in a shared client.
- QR, nav and index-info endpoints have different envelope shapes. Validate each endpoint's business result, not just HTTP 200 or the presence of `data`.
- Optional profile enrichment cannot invalidate successful identity/root verification. Missing or malformed optional capacity is unknown, not zero or proof of expired credentials.
- Optional composite fields must be decoded into temporary values and committed only on success: a JSON error can leave partial values populated. A malformed privilege object must not leave `state=true` as trusted membership evidence.
- Keep normal TLS verification. Reference code is not permission to copy plaintext mirrors, certificate bypasses or raw response logging.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
| --- | --- |
| `path` ends at a different directory / silently falls back to root | Reject the response; never display root as the requested directory |
| Malformed JSON or missing required identity fields | Upstream/protocol error, not an empty successful page |
| Explicit login failure | `ErrCredentialInvalid` |
| HTTP rate limit / upstream busy response | Safe classified failure; do not mislabel the account as invalid |
| Caller cancellation / network failure | Abort the request; no raw URL, Cookie or challenge in returned error |
| Unknown QR status / missing returned Cookie | Error, not fabricated waiting or successful login |

### 5. Good/Base/Bad Cases

- Good: repeated responses with `cid: 123` and `cid: "123"` produce the same directory and breadcrumbs.
- Base: persisted `UID/CID/SEID/KID` credentials continue to work without a database migration.
- Bad: decode `paths[].file_id` into `int`, ignore `json.UnmarshalTypeError` and use partially populated zero IDs.

### 6. Tests Required

Use a simulated HTTP upstream to assert mixed/large IDs, repeated directory requests, invalid-directory root fallback, all QR states/devices, existing credential JSON, optional profile fields, cancellation, concurrent Cookie isolation and safe errors. Run backend vet/tests, adapter race tests and `make check`. These checks do not establish live 115 acceptance.

### 7. Wrong vs Correct

Wrong: parse an ID into `float64` and format it back, or use decoded names after any JSON error.

Correct: normalize a raw integer/string into exact decimal text, reject unsupported types and return the parsing error before using any partial response.
