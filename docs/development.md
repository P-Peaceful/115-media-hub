# 开发说明

## 初始化边界

已有：前后端工程配置、健康检查、SQLite 连接与 goose 迁移、JSON 日志与轮转、信号退出、静态资源与 SPA 回退、单管理员登录/内存会话/CSRF 校验、受保护路由基础、主题切换、React Router、TanStack Query Provider、Zustand 界面偏好、OpenAPI 生成、Docker 联合构建。

已实现：115 扫码授权与单连接管理、凭据明文保存、只读连接测试和非敏感账号资料快照。侧栏设置下方展示授权账号、会员概况与存储用量，可在连接页刷新资料。连接管理仅供已登录管理员使用；二维码挑战由后端短时内存持有，前端刷新后需重新发起授权。未实现：115 文件浏览、媒体库、扫描、STRM、播放解析、任务调度/持久化执行/SSE 事件、海报墙、元数据回传、审计、备份恢复。cron 等业务阶段依赖届时再引入；Motion、TanStack Table/Virtual 按需引入。

## 配置

优先级：默认值 < `-config` 指定的 YAML < 环境变量。

| 环境变量 | 默认值 |
| --- | --- |
| `MEDIA_HUB_HTTP_ADDR` | `127.0.0.1:8080` |
| `MEDIA_HUB_DATABASE_PATH` | `./data/115-media-hub.db` |
| `MEDIA_HUB_LOG_DIR` | `./logs` |
| `MEDIA_HUB_SHUTDOWN_TIMEOUT` | `10s` |
| `MEDIA_HUB_ADMIN_USERNAME` | 必填；也可在 YAML 配置 `admin_username` |
| `MEDIA_HUB_ADMIN_PASSWORD` | 必填；也可在 YAML 配置 `admin_password` |
| `MEDIA_HUB_COOKIE_SECURE` | `false`；HTTPS 部署设置为 `true` |

相对路径以进程工作目录为基准。根目录 `.env.example` 是配置说明；Go 不自动加载 `.env`。Vite 代理目标通过启动环境变量 `MEDIA_HUB_DEV_API_URL` 指定，例如 `MEDIA_HUB_DEV_API_URL=http://127.0.0.1:9090 make dev-frontend`。

YAML 使用严格字段校验，未知字段、非法端口、空数据库路径、非正退出超时和缺失/全空白的管理员用户名或密码会阻止启动。旧 YAML 中残留的 `credential_master_key` 与 `credential_key_version` 仅为兼容读取而忽略，不再作为应用配置；建议从本机配置中删除。用户名和密码没有默认值。ENV 按字段覆盖 YAML；显式设置为空仍会覆盖 YAML 并导致启动失败。密码以原文验证，首尾空格也属于密码内容，但全空白密码不允许。修改凭据需更新部署配置并重启，重启会清空全部内存会话。

本地可设置 `MEDIA_HUB_ADMIN_USERNAME` 和 `MEDIA_HUB_ADMIN_PASSWORD` 后运行 `make dev-backend`，或在 `backend/configs/config.example.yaml` 复制并填写 YAML 字段。Docker Compose 通过必填环境变量注入用户名和密码：复制 `.env.example` 为 `.env`，取消凭据变量注释并填写强密码，再运行 `docker compose -f deploy/compose.yaml up -d`。修改 `.env` 后重建容器以应用配置。默认 Cookie Secure 关闭以支持回环 HTTP；通过 HTTPS 提供服务时设置 `MEDIA_HUB_COOKIE_SECURE=true`，应用不根据转发头推断 TLS。凭据不要提交到版本库。

115 Cookie 凭据以明文 JSON 保存在服务端 SQLite 的 `connections.credential_json` 中，不会返回前端、写入 URL 或日志。非敏感账号/会员/容量快照保存在独立的 `connections.profile_json` 中；资料缺失不会使连接失效。数据库文件权限是 0600，但明文凭据仍会随数据库备份和复制文件一同保存；限制数据库、备份和主机访问权限，并按敏感凭据保护备份。无需配置凭据主密钥。

## 115 授权操作

先登录管理后台，在设置下方的 115 授权卡片进入连接管理。选择 Web、支付宝小程序或微信小程序后生成二维码，并在 115 客户端确认；Web 设备可能影响同账号现有网页登录，也可能触发 IP 登录异常。二维码有时效，过期、取消或页面刷新后重新生成即可，未完成的会话不会创建连接。授权完成后会验证账号身份与根目录读取能力；账号资料可在连接页手动刷新，资料刷新失败不会删除快照或撤销连接。

使用“只读测试连接”检查凭据与根目录能力。若凭据失效，使用“重新授权”并登录原来的 115 账号；同账号授权会保留连接 ID 并替换凭据。若授权账号与现有连接不同，系统会拒绝替换并保留原连接，请先确认 115 登录账号。当前版本不提供断开/删除连接。数据库及备份包含可用 Cookie 凭据，应限制读取权限并在复制、迁移和恢复时按登录凭据保护。

## 数据库和日志

数据库必须位于本地磁盘，启用 WAL、foreign_keys、5000ms busy_timeout，并将连接池限制为一个连接。只允许 goose 版本化 SQL 修改结构，不使用 GORM AutoMigrate。当前迁移创建 goose 版本管理表和 `connections` 业务表；115 活动连接由部分唯一索引限制为一个，凭据列保存服务端使用的明文 JSON Cookie 凭据。

数据库文件以 0600 创建，目录以 0700 创建。日志写入标准输出与 `app.log`，轮转阈值 20 MiB、最多 5 个备份、保留 14 天、压缩旧文件。HTTP 日志仅记录方法、匹配路由、状态和耗时，不输出原始 URL、请求体、Cookie 或凭据。未来业务日志仍须遵循脱敏约定。

## 前端

HeroUI v3 使用 `@heroui/react` 与 `@heroui/styles`，不需要 v2 的 Provider。CSS 导入顺序为 Tailwind → HeroUI → 应用变量。主题仅存储在 Zustand 偏好中；服务端数据归 TanStack Query；导航目录、筛选、分页应使用 URL。

请求封装只接受同源绝对路径，使用 same-origin Cookie；登录后 CSRF token 仅保存在运行时内存并自动附加到变更请求。管理员会话为单实例内存态，随机会话 ID 通过 HttpOnly、SameSite=Lax Cookie 传输，自创建后固定 24 小时过期且不滑动续期；活动请求不延长寿命，重启或退出立即失效。`/api/auth/login`、`/api/auth/session`、`/api/auth/logout` 是认证端点；未来管理 API 应通过 `RegisterManagementRoutes` 注册以要求有效会话和 CSRF token。健康探针保持公开；115 播放认证不在此范围内。

## 契约与构建

修改 `api/openapi.yaml` 后运行 `make generate`，提交契约及两端生成文件。Go 健康检查和认证路由通过生成的 ServerInterface 注册，避免手工路由与契约漂移。

`make build` 将 `frontend/dist` 复制到 `backend/internal/webui/dist`，以 `embedui` build tag 构建生产程序。默认 Go 开发构建无此前端资源依赖。产物目录不提交。

根目录 Makefile 是跨工程入口，不需要 Go workspace 或 pnpm workspace。Go module 名称暂使用 `github.com/wenzhengcheng/115-media-hub/backend`；若实际远端地址不同，后续统一替换 module 与导入路径。

## 验证

`make check` 检查格式、Go vet、Go 测试、前端类型、ESLint、peer 依赖及 Vitest。`make smoke` 使用临时目录启动真实二进制，检查健康接口、嵌入首页、SPA 深链、API/静态资源 404、数据库无业务表和 SIGTERM 退出，测试结束后清理临时数据。

Docker Compose 默认仅绑定宿主机回环地址。运行用户为 `mediahub`，UID/GID 为 10001；改用 bind mount 时应确保目录可写，隔离区必须在媒体扫描范围之外。恢复和清理业务尚未实现。

本地 Makefile 使用 `-buildvcs=false`，支持无可用 Git 元数据的源码目录。项目环境变量前缀为 `MEDIA_HUB_`，默认数据库文件名为 `115-media-hub.db`。
