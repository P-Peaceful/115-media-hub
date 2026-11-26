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

附属文件和播放取链都使用同一 Cookie 账号配额，但操作类型分别是后台 `DownloadLink` 和前台 `Playback`。Cookie 下载取链按固定顺序尝试：

1. `POST https://proapi.115.com/android/2.0/ufile/download`，加密载荷字段为 `pick_code`；
2. `POST https://pro.api.115.com/android/2.0/ufile/download`，同上；
3. `POST https://proapi.115.com/app/chrome/downurl`，兼容载荷字段 `pickcode`，带时间查询参数；
4. `POST https://webapi.115.com/files/download`，加密载荷字段 `pickcode`；
5. `GET https://webapi.115.com/files/download?pickcode=...`。

每个取链通道最多实际发送一次，单次上游预算 5 秒；授权失败、明确文件缺失、限流、取消、凭据变更和不可信目标不得回退，协议/临时网络失败才可进入下一个通道。Android/Web POST 的解密结果是单 URL 或 `file_url`，Chrome 结果按源文件 ID 索引并核对 `pick_code`，Web GET 兼容顶层或嵌套 `url`。取链目标只在内存中保留签发 UA 和响应 `Set-Cookie`。

取链完成后释放 API 配额，内容流由账号级最多 2 个并发、16 个等待槽的独立有界队列处理。NFO 上限 8 MiB、图片上限 64 MiB，单文件总预算 120 秒、单次内容请求 30 秒，普通临时错误最多 3 次；连续 CDN 403 仅可在该预算内尝试最多 4 个去重的 UA/Cookie 组合。登录 Cookie 只允许发送到 `115.com` 账号域；`115cdn.com`/`115cdn.net` 只接收取链响应下发且域名匹配的临时 Cookie，跨主机重定向后永久移除 Cookie。CDN `Retry-After` 冷却共享且不能缩短，HTTPS/可信 115 域和每次重定向均需校验。播放 302 不下发或转发必须 Cookie 的 `f=3` 直链。

### 下载接口矩阵的代码规范

#### 1. Scope / Trigger

- 触发：115 私有下载接口存在 Chrome、Android 和 WebAPI 多种协议，附属文件同步必须在取链失败时有受控回退。

#### 2. Signatures

- `DownloadSidecar(ctx, DownloadRequest, DownloadSink) (DownloadReceipt, error)` 使用后台 `DownloadLink` 取链和内容队列。
- `ResolvePlayback(ctx, accountID, sourceFileID, playbackCode, userAgent, requestID) (string, error)` 只返回不需要额外 Cookie 的播放地址。

#### 3. Contracts

- 所有取链请求经过 `internal/adapter/provider115` 的固定端点白名单、账号冷却和实际发送预算。
- `pick_code`、`pickcode` 只在对应协议编码器中生成；不得把一个端点的响应解析器用于另一个端点。
- 下载头和 Cookie 不进入日志、STRM、数据库、任务错误或公共 API。

#### 4. Validation & Error Matrix

- 认证、限流、明确文件缺失、取消、凭据变更 → 保留稳定 `errors.Is` 身份并停止回退。
- 协议无效、临时 HTTP/网络失败 → 按固定顺序进入下一取链端点。
- 不可信 URL、跨域重定向或无 URL 响应 → 拒绝，不回退到其他下载目标。

#### 5. Good / Base / Bad Cases

- Good：Android 端点失败后 Web POST/GET 成功，CDN 403 使用签发响应 Cookie 成功。
- Base：Chrome 接口直接成功，或 CDN 不返回 Cookie 时无额外组合请求。
- Bad：把登录 Cookie 发往 `115cdn.*`，或把 `pickcode` 发送到 Android `pick_code` 接口。

#### 6. Tests Required

- 覆盖五通道顺序、每通道一次、账号间隔、字段拼写、四种 CDN 403 组合、跨主机 Cookie 清理、`f=3` 播放拒绝、错误停止回退和日志脱敏。

#### 7. Wrong vs Correct

- Wrong：只调用 `/app/chrome/downurl`，失败后直接返回 `sidecar_take_link_failed`。
- Correct：按固定端点矩阵受控回退，并将已识别的授权、限流、文件缺失和协议错误分别保留。
