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
- Connection summaries may contain a separate safe profile snapshot with account name, membership status/expiry, storage totals in bytes, and retrieval time. Internal SDK level numbers are evidence for status and legacy-snapshot repair only; do not present them as user-facing VIP levels or map them to official names without a source mapping. Unknown fields stay absent/unknown; they must not be inferred as zero or ordinary membership.
- The pinned Cookie SDK decodes `vip` as a plain integer without preserving whether the field was present. A positive level with no elapsed expiry is evidence of VIP; zero or elapsed level data alone is unknown, not proof of an ordinary account. Only emit active membership from a positive current level or explicit active privilege signal.
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

Keep the third-party SDK types and error strings inside the adapter. Add public API changes to `api/openapi.yaml`, then regenerate both Go and TypeScript contracts.

For account membership, the pinned Cookie SDK exposes numeric `UserInfo.Vip` but no `vip_info.level_name`. Use a positive, unexpired value only as evidence that membership is active; the UI shows VIP status and expiry, not the raw number. Do not infer official labels such as `年费VIP` from that integer. The Open API `level_name` field belongs to the documented Bearer flow and is unavailable from the current QR Cookie credential.
