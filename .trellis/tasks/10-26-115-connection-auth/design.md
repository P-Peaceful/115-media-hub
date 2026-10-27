# 115 扫码授权与连接管理技术设计

## Intent

把 115 Cookie 扫码授权封装成一个可替换的后端 adapter，并通过受保护的管理 API 提供二维码会话、状态轮询、完成授权和连接测试。最终只把连接摘要返回前端；`UID/CID/SEID/KID` 留在后端，以明文 JSON 保存在 SQLite；不实现凭据加密或主密钥配置。数据库与备份按敏感登录凭据保护。

## Approaches considered

1. Cookie 扫码 + 项目 adapter（采用）：不要求开放平台 AppID，GitHub 上有成熟的 Go 实现，能直接覆盖后续私有文件 API。代价是依赖非稳定公开接口，需要设备提示、错误分类和重新授权。
2. 官方开放平台 OAuth：凭据边界更规范，支持 access/refresh token；但需要自有应用和回调/PKCE，公开项目的移动端扫码仍不完整，暂不作为首版。
3. 借用第三方 AppID 或公共回调服务（不采用）：部署简单，但增加第三方信任、可用性和配额依赖，不符合单实例自托管边界。

## Architecture and boundaries

### Adapter

`backend/internal/adapter/drive115` 定义项目自己的接口和类型：

- `StartQRCode(ctx, device) (Challenge, error)`
- `PollQRCode(ctx, challenge) (QRCodeStatus, error)`
- `CompleteQRCode(ctx, challenge, device) (Credential, error)`
- `VerifyCredential(ctx, credential) (AccountIdentity, CapabilityEvidence, error)`

适配器内部可固定依赖 MIT 许可的 `github.com/SheltonZhu/115driver`，但第三方类型、HTTP 响应和错误不能越过 adapter。默认 HTTP 客户端必须有总超时；所有调用都接收 `context.Context`。首版设备枚举为 `web`、`alipaymini`、`wechatmini`，默认 `web`。

### Domain and service

`backend/internal/domain` 保存设备、二维码状态、连接状态、账号身份、能力证据和凭据值对象。`backend/internal/service` 负责：

- 创建有过期时间的内存扫码会话，并生成不透明本地 session ID；只保存 115 challenge，不保存前端请求上下文。
- 将第三方状态映射到 waiting、scanned、allowed、expired、canceled、failed。
- 在 allowed 后完成换取凭据、验证账号/根目录读取能力、检查已有连接账号一致性，然后短事务写入连接。
- 同账号重新授权沿用连接 ID，替换凭据 JSON 并清空旧能力证据；不同账号返回冲突且不修改数据库。
- 连接测试只读外部能力，不在事务内执行网络调用；网络错误分类为 invalid、limited、unreachable、unknown 等，不能直接标记凭据失效。

内存扫码会话仅保存有限数量和短 TTL；过期、取消、完成和服务停止时删除。页面刷新不会恢复二维码会话，管理员需要重新发起；这避免把 115 challenge 放进 URL 或浏览器持久化存储。

### Persistence

新增 goose migration 创建 `connections` 表，沿用数据库设计中的字段：`id`、`kind`、`name`、`external_account_id`、连接地址字段、`credential_json`、`capabilities_json`、`is_active`、`status`、`retry_after_at`、时间戳，以及一个 115 活动连接的部分唯一索引。按项目数据库规范不添加外键或 CHECK；Service 在事务内校验单例、状态、JSON 版本和账号一致性。

按用户明确决定，`credential_json` 直接保存 `{UID,CID,SEID,KID}` JSON，不提供加密密钥配置。HTTP API、前端、本地 URL 和日志仍不得回显 Cookie 或凭据；数据库及备份含可用的 115 登录凭据，需要限制文件和备份访问权限。

### HTTP contract

在 `api/openapi.yaml` 增加受保护管理端点并运行 `make generate`：

- `GET /api/admin/connections/115`：返回当前连接摘要或 `configured=false`。连接摘要只含非秘密账号信息、状态和能力证据。
- `POST /api/admin/connections/115/qr-sessions`：请求设备类型，返回本地 session ID、二维码内容/图片来源、过期时间和初始状态。
- `GET /api/admin/connections/115/qr-sessions/{sessionId}`：返回规范化扫码状态；不返回 `uid/time/sign` 或第三方原始响应。
- `POST /api/admin/connections/115/qr-sessions/{sessionId}/complete`：只在 allowed 后完成换取、验证和明文入库，返回连接摘要。
- `POST /api/admin/connections/115/test`：对已保存凭据执行只读连接测试，返回能力证据和状态。
- 115 调用失败时，API 的稳定错误对象可携带 `upstreamCode`；`message` 由 adapter 从上游错误对象提取可读字段并脱敏。轮询仍返回规范化 failed 状态，并在 `message`/`upstreamCode` 传递上游诊断信息；测试接口将诊断消息放在结果中。不得透传原始 body 或 HTTP headers。

所有端点挂到现有 `RegisterManagementRoutes` 管理组，变更请求使用已有 CSRF 机制。错误只返回稳定类别和可读消息，不回显上游响应。

### Frontend

新增 `frontend/src/features/connections` 页面和路由。TanStack Query 管理连接摘要和扫码状态；设备选择、当前扫码 session 和轮询错误只放内存，不进入 Zustand、URL、localStorage 或 sessionStorage。轮询间隔有上限，遇到 expired/canceled/failed 停止；allowed 后调用 complete 并刷新连接摘要。页面提供默认 Web、支付宝小程序、微信小程序选择，Web 设备风险提示，连接测试和重新授权按钮。二维码内容只用于当前组件渲染。

连接管理页面保留为 `/connections`，但不作为主侧边栏独立导航项。设置项下方的 `ConnectionSidebarCard` 复用同一个连接摘要 Query，未授权时引导授权，已授权时展示账号名、会员状态与已用/总容量，并可进入连接管理页。连接详情显示完整的安全账号资料快照，并通过受保护的 profile refresh API 手动更新；未知字段显示为未知，不把资料读取失败等同于连接失效。

### Account profile contract

Adapter 在验证 Cookie 时尽力读取 115 用户、头像、会员、容量和 SDK 返回的已登录设备信息；容量读取失败时保留身份信息并让容量字段为空。用户提供的 `/open/user/info` 文档使用 Bearer `access_token`，本项目扫码流程只有 Cookie 凭据，因此不得据此直接调用，也不得伪造 Cookie 接口未返回的 VIP 名称或第三方权益。UI 用连接身份显示 UID、标记 Cookie 授权方式；SDK 原始数值只用于判断会员状态，不作为会员等级文案展示。SDK 未返回官方 `level_name` 时不推导等级名称，展示可确认的 VIP 状态和到期时间。

Cookie SDK 的 `vip` 字段是无 presence 标记的整数：正等级且未过期可确认 VIP；零值或已过期的等级本身不足以确认普通会员，因此展示未知。明确有效的 privilege 状态或长期权益字段仍可作为有效会员证据。

profile 是非凭据快照，单独存放在 `connections.profile_json`，以 `schema_version` 包装并包含抓取时间。新增 goose migration，不改动已发布的旧迁移。`Connection` API 返回用户名、可验证的 HTTPS 头像、会员状态/原始等级/到期时间、总/已用/剩余字节数、设备数和抓取时间；Cookie 字段仍只存在 `credential_json`。前端仅允许预期 115 图片域名的头像并设置 no-referrer，其他值回退到账号首字母。

管理端点 `POST /api/admin/connections/115/profile/refresh` 重新读取并持久化 profile；它不替换凭据或连接身份。刷新失败不会清除快照、改变连接状态或覆盖登录凭据。未知会员状态和缺失容量是显式的未知值，不推断为普通用户或 0 字节。

## Data flow

1. 管理员打开连接页，查询连接摘要。
2. 选择设备并创建 QR session；Service 调用 adapter 获取 challenge，存入短期内存会话。
3. 前端先加载受保护的二维码图片，再轮询本项目状态端点，避免轮询先结束会话并清掉尚未取回的图片。
4. 115 状态进入 allowed 后，前端调用 complete；Service 换取 Cookie、验证账号与根目录能力。
5. Service 在短事务内检查单例和账号一致性，序列化凭据 JSON，写入/更新连接并清理内存 challenge。
6. 前端只收到连接摘要和能力证据；后续文件模块通过 connection ID 请求 Service，再由 adapter 读取凭据并调用 115。

## Compatibility, operations and rollback

- 不新增凭据主密钥配置；现有无业务表数据库通过迁移创建 `connections`。此前未发布的加密凭据格式不做兼容，需重新授权后保存明文 JSON。
- 不支持此前未发布的 AEAD 密文记录（需重新授权）；不提供删除连接，不改变管理员会话和播放凭据。
- 115 上游协议变化时只修改 adapter 和错误映射；Service、OpenAPI 与前端状态保持稳定。
- 回滚需要同时撤销 OpenAPI 生成物、路由、迁移和配置文档；不要通过删除已有数据库文件回滚。连接表迁移保留，旧版本不识别新表但可继续启动。

## Security and testing notes

- 使用 httptest 和 fake adapter 覆盖二维码状态转换、过期/取消、重复完成、账号冲突、凭据 JSON 持久化、API 脱敏和网络错误分类。
- 使用临时真实 SQLite 验证迁移、部分唯一索引、短事务和回滚；测试日志与 HTTP 响应不含 cookie/token sentinel。
- 前端测试覆盖设备选择、Web 提示、轮询停止、页面刷新重新开始、401/403 和连接摘要脱敏。
