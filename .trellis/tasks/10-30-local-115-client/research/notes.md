# Reference evidence

Retrieved 2026-10-30 using GitHub connector from pancras-loe/115-station default branch. Source snapshots were used for research only and moved to /tmp/115-station-local-client-reference after review; durable source links and protocol findings are recorded below.
- internal/api/115.go: token/image/status/result URLs; three supported app mappings; status 0/1/2/-1/-2; consistent 115Browser UA and Referer.
- internal/api/panpath.go: rawStr json.RawMessage to string for IDs, /files limit=1 hide_data=1 path ancestors, verify final cid and aid.
- internal/api/files115.go: web field meanings (f=0 directory, cid=self for directory, fid=file), HTTP and business checks, mirror origins, per-request Cookie and throttle.
- internal/api/cap115.go: capacity may be decimal/scientific JSON numbers or strings; truncate finite nonnegative bytes and reject overflow. ID parsing remains strict decimal integer text.
- internal/api/routes.go (selected excerpts): my.115.com/?ct=ajax&ac=nav user identity, files/index_info optional capacity and login_devices_info. Never ignore malformed identity decoding or fabricate a profile on failure.
- Source URLs: https://github.com/pancras-loe/115-station/blob/master/internal/api/115.go ; https://github.com/pancras-loe/115-station/blob/master/internal/api/panpath.go ; https://github.com/pancras-loe/115-station/blob/master/internal/api/files115.go
Do not port its Handler, model, certificate exceptions, plaintext HTTP mirrors, full raw response logging, or error-as-waiting behavior. Adapt protocol to this repository's context, privacy and error contracts.

## User steering: upstream membership text (2026-10-30)

User requests account fields directly reflect upstream and membership be upstream text rather than numeric level. Cookie API `https://webapi.115.com/user/vip_limit?feature=2` is documented by p115client `user_vip_limit` (https://p115client.readthedocs.io/en/latest/reference/module/client.html#p115client.client.P115Client.user_vip_limit). Primary client source demonstrates `data.user_limit.level_name` as the display name: https://greasyfork.org/en/scripts/531685-一键复制磁力链和推送到115离线/code (lines 1991–2035 in indexed source). This differs from Bearer-only `vip_info.level_name`; do not add Bearer auth or guess nav fields. Preserve name verbatim as text, no numeric-label mapping or name regex-derived membership status. This optional lookup cannot invalidate identity/root verification. Never expose an entire raw upstream payload.
