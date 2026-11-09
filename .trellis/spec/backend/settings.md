# Settings and 115 authorization contract

## 1. Scope / trigger

Settings cross the SPA, management API, authentication service, SQLite and the 115 upstream. `api/openapi.yaml` is the canonical transport contract. User chose one personal 115 account and explicitly excluded encryption/master-key protection for this credential; do not extend that choice to unrelated providers.

## 2. Signatures

- `GET /api/v1/settings/security`, `PUT /api/v1/settings/security/credentials`, `PUT /api/v1/settings/security/session`.
- `GET|DELETE /api/v1/settings/115`, `GET /api/v1/settings/115/details`, `POST /api/v1/settings/115/qr`, `POST /api/v1/settings/115/qr/{id}/poll`, `DELETE /api/v1/settings/115/qr/{id}`.
- Migration `00003_115_account.sql` owns singleton `provider_115_account`, including plaintext cookie; downgrade rejects irreversible data removal.

## 3. Contracts

All require an administrator session and state-changing calls require the existing CSRF proof. Credentials update takes `username`, `current_password`, and `new_password` (empty retains password); successful change retains the calling session and revokes others. `session_lifetime_days` accepts 1, 7, 30 and affects future sessions, not existing expiry. Existing login `remember_login` remains per-login.

QR response includes opaque id, PNG data URI, status and expiry; statuses are waiting, scanned, succeeded, expired, canceled. Temporary state is bound to the initiating session. Confirmation exchanges the upstream result server-side and replaces the singleton only on success; no account identity comparison. Account response exposes connection metadata, never cookie. Details and explicit authorization checks use the same stored Cookie and matching 115 Browser User-Agent. `GET /api/v1/settings/115/details` freshly requests `https://my.115.com/?ct=ajax&ac=nav` and `https://webapi.115.com/files/index_info`; a user-triggered check forces this request rather than trusting cached UI data. This is the Cookie-based app flow, not OpenAPI OAuth/Bearer. The profile projection may expose user name, user ID, avatar, nullable `is_member`, nullable `member_expires_at`, and nullable `member_forever`. The observed Cookie `nav` response uses `vip=0` for non-member and `vip=1|2` for member in the reference implementation; map known values only to a boolean and keep unknown values unknown. `expire` is a Unix-seconds timestamp and `forever` indicates permanent membership. Never display raw `vip` as a membership tier. `files/index_info` also provides `login_devices_info.list` with name, device type, IP, city, login timestamp (`utime`) and current-device flag (`is_current`); expose devices with a separate availability state if parsing is unavailable. The response distinguishes unconfigured, valid and invalid authorization, and independently reports capacity/device availability. Provider user IDs and byte counts are decimal strings to avoid JavaScript precision loss. An explicit login-required `nav` response means invalid authorization; transport, parse, or unrelated upstream errors mean status cannot be confirmed and must not be reported as expired. Replace/remove during an in-flight read must not return stale account data. These endpoints and mappings are observed provider interfaces, not a guaranteed stable public contract. Deletion removes local authorization. No SDK or master key is required. Never emit cookie or QR secret in logs. SQLite and backups contain the plaintext 115 credential.

## 4. Validation and errors

Malformed or invalid fields: 400; incorrect current password: 401; missing/revoked session: 401; invalid CSRF/origin: 403; unknown/expired QR session: 404; upstream/storage failure: 503. Failure or cancellation cannot replace existing authorization. Consult the OpenAPI error envelope for machine codes.

## 5. Good / base / bad

- Good: scan and confirm → save credential once → account metadata remains after restart.
- Base: failed or canceled new scan → previous account remains; capacity lookup fails → valid account with unavailable capacity, not zero capacity.
- Bad: compare a password then issue a stale session after a concurrent credential change; return a Cookie to the browser; let an older QR completion restore authorization after cancellation/removal.

## 6. Tests required

Use real SQLite for migration, audit rollback, credential/session revocation and login races; mock upstream for QR state, errors, expiry, cancellation and replacement races; also test profile/avatar/member/expiry mapping, device-list parsing, capacity parsing, invalid credentials, upstream errors, large numeric values, and credential replacement during a details/check request. Verify manual checks distinguish accepted, explicitly expired, and inconclusive authorization; verify CSRF and session ownership, structured business outcomes and redaction, and frontend validation/state changes. Report actual mobile-app scan separately from mock tests.

## 7. Wrong vs correct

Wrong: return the provider cookie in the QR response, treat a network timeout as proof that authorization expired, or expose the raw `vip` integer as a level. Correct: exchange server-side, map known `vip` values only to member/not-member, return only needed account metadata, distinguish inconclusive checks from invalid authorization, and surface failures without overwriting prior authorization.
