# 115 扫码授权与连接管理实施清单

## 1. 配置与数据库

- [x] 用户决定移除凭据加密及主密钥/key version 配置；清理 YAML/ENV、部署示例和开发文档。
- [x] 用户决定凭据以明文 JSON 持久化；移除 envelope 与加解密实现，保持 HTTP/日志边界脱敏。
- [x] 新增 goose migration 创建 `connections` 表和 115 活动连接部分唯一索引；实现 `repository/sqlite/model` 与 repository 查询/更新。
- [x] 为迁移、凭据 JSON 存取、账号冲突和事务回滚编写 Go 测试。

## 2. 115 adapter 与 Service

- [x] 固定并引入经审查的 MIT `115driver` 版本；仅在 `internal/adapter/drive115` 内使用其类型。
- [x] 实现 `StartQRCode`、`PollQRCode`、`CompleteQRCode`、`VerifyCredential`，设置 HTTP 超时、context 取消和安全错误映射。
- [x] 实现内存 QR session manager：不透明 ID、短 TTL、数量上限、设备绑定、状态转换、重复请求幂等和完成后清理。
- [x] 实现连接 Service：连接摘要、扫码生命周期、账号一致性、凭据 JSON 持久化、能力证据、只读连接测试和重新授权覆盖。
- [x] 用 fake adapter 覆盖 waiting/scanned/allowed/expired/canceled/failed、网络不可达、凭据失效和重复 complete。

## 3. API 契约与后端 HTTP

- [x] 修改 `api/openapi.yaml`，增加连接摘要、设备枚举、QR session 状态、能力证据、稳定错误类别和管理端点。
- [x] 运行 `make generate`，检查 Go generated server interface 与前端 schema；不手改生成文件。
- [x] 新增薄 HTTP handler 和 router composition，保护管理 API 并复用管理员 session/CSRF。生成路由使用固定 `/api/admin/...` 路径，因此 router 通过受保护前缀中间件统一应用现有 auth/CSRF。
- [x] 用 `httptest` 覆盖未认证、CSRF、非法设备、状态轮询、重复完成、账号冲突和不泄密错误响应。

## 4. 前端连接页

- [x] 实现 `frontend/src/features/connections` 页面、Query hooks 和本地扫码状态；加入受保护路由与导航入口。
- [x] 支持默认 Web、支付宝小程序、微信小程序；Web 风险提示清晰可见，二维码内容只在当前内存组件中使用。
- [x] 实现创建、轮询、完成、连接测试、重新授权、过期/取消/失败和会话失效状态；刷新页面后允许重新发起扫码。
- [x] 覆盖原生键盘可访问语义（fieldset/legend/radio label）、加载中、未知/部分能力、401/403、轮询停止和摘要不含秘密；组件渲染用 SSR 断言，API 边界另有请求测试。
- [x] 移除独立连接管理侧栏导航；设置下方卡片显示 115 授权状态与账号摘要并可进入 `/connections`。
- [x] 连接页展示会员与容量资料，支持手动刷新并保留未知/部分资料状态。

## 4.1 账号资料摘要

- [x] 在 adapter 边界映射账号名、会员状态/等级/到期时间与可用容量；容量读取失败不得阻断已验证连接。
- [x] 增加独立 `profile_json` 迁移，更新连接摘要 API/OpenAPI 生成物，禁止返回凭据字段。
- [x] 增加受保护的资料刷新端点，刷新只更新 profile snapshot，不更改连接凭据或连接状态。
- [x] 依照 115 用户信息展示参考补充头像、UID、授权方式、会员到期与容量进度；使用 SDK 可读的会员/设备数据，未提供的 VIP 名称和第三方权益保留为未知/不展示。
- [x] 115 调用错误通过统一的安全错误类型提取并脱敏后传到前端；覆盖 HTTP 错误和扫码状态轮询消息，禁止透传原始响应 body/headers。

## 5. 文档与验证

- [x] 更新 `docs/development.md`、`.env.example`、`backend/configs/config.example.yaml` 与部署说明，说明明文凭据存储、数据库/备份保护、扫码设备、Web 风险和重新授权覆盖。
- [x] 更新项目上下文或连接规范，记录 adapter、明文凭据存储和单活动连接约束。
- [x] 运行 `make generate`、`make check`、`make smoke` 和 `git diff --check`；均通过。smoke 的数据库断言同步支持 `connections` 表。真实 115 账号联调未执行。

## Risk checkpoints

- 上游二维码端点或状态码变化：先在 adapter 增加 fixture/fake，禁止把原始状态泄漏到 API。
- 第三方 SDK API/许可变化：固定版本并核对 MIT 许可，保留替换 adapter 的边界。
- 明文凭据存储：确保凭据仅留在服务端与 SQLite，不进入 API、日志、任务快照或 URL；限制数据库与备份权限。
- 账号冲突：在写入前验证，确保旧凭据 JSON 和连接摘要保持不变。
