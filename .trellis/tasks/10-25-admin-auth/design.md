# 技术设计：单管理员登录与管理入口保护

## 目标与边界

在当前 Gin + React SPA 基础上实现单管理员认证闭环。凭据来自启动配置，服务端会话仅驻内存；认证不依赖数据库，不增加账户、角色或会话表。健康探针保持匿名可用，播放认证保持独立且不在本任务实现。

## 架构与职责

- `internal/config`：新增 YAML `admin_username`、`admin_password` 和 ENV `MEDIA_HUB_ADMIN_USERNAME`、`MEDIA_HUB_ADMIN_PASSWORD`。沿用默认值 < YAML < ENV 的逐字段优先级；用 `os.LookupEnv` 保留显式空 ENV 的覆盖语义。启动校验拒绝缺失或全空白凭据，不在错误信息中带出值。
- 新的认证/会话包：持有配置凭据、随机会话 ID、随机 CSRF token 和到期时间；会话生命周期为创建后固定 24 小时。所有随机值使用 `crypto/rand`。查找时拒绝过期会话并惰性清除；后台清理可选，需避免无界残留，且不能改变到期语义。凭据及 token 比较使用常量时间比较。
- HTTP handler/router：认证接口放在 `/api/auth`：`POST /login`、`POST /logout`、`GET /session`。登录成功轮换/新建 session ID，设置 HttpOnly Cookie，并在响应体返回 CSRF token；当前会话查询验证 Cookie，返回认证状态及 CSRF token；退出要求有效会话和 CSRF token，清除服务端会话并过期 Cookie。未登录/过期返回统一未认证响应。未来管理 API 在明确受保护的路由组注册；`/health/live`、`/health/ready` 和认证入口不被管理认证中间件拦截。
- CSRF：所有受保护的非安全方法（除登录外）强制要求有效会话及 `X-CSRF-Token`；比较 token 时常量时间比较。CORS 不开放跨源凭据。登录不要求已认证的 CSRF token；登录失败不签发会话。Cookie 的 SameSite=Lax 是纵深保护，不代替 CSRF 检查。
- Cookie：随机会话 ID 仅通过 Cookie 传输，设置 `HttpOnly; SameSite=Lax; Path=/`，Max-Age 与剩余绝对寿命一致。新增可选 `cookie_secure` / `MEDIA_HUB_COOKIE_SECURE` 布尔配置，默认 false 以兼容当前 HTTP 回环和 Compose 回环绑定；通过 HTTPS 服务时部署者必须显式启用。不能信任未经配置的 `X-Forwarded-Proto`。
- `api/openapi.yaml` 是 API 契约来源；运行 `make generate` 更新 Go server interface 和前端 schema。生成代码不手改。
- React 前端：增加认证状态 owner（Query 或专用 provider，避免另建持久化 store）、登录页面和路由保护。登录页按 Canva《115 Media Hub — UI 设计 V1.0 · 38 个视图》呈现“欢迎回来”、管理员账号、密码、部署凭据说明，并说明 115 授权在管理登录后单独配置。认证加载中先显示占位，不渲染受保护内容；未认证时记录安全的站内目标和非敏感 query 后转登录。认证秘密只保存在运行时内存，禁止 localStorage/sessionStorage/URL/log。现有 `request` helper 继续使用 same-origin Cookie 与显式 CSRF token；会话初始化从 `/api/auth/session` 获取 token。
- 配置和部署文档：更新 `backend/configs/config.example.yaml`、`.env.example`、`deploy/compose.yaml` 与开发说明，展示 ENV/YAML 配置和修改后重启的重置流程。Compose 示例不提供默认生产凭据，需由部署者注入变量。

## 请求与数据流

1. 进程启动时加载 YAML/ENV 并拒绝无效管理员凭据，凭据不写日志或数据库。
2. 未认证 SPA 请求 `GET /api/auth/session` 得到未认证状态；用户提交用户名/密码到登录接口。
3. 后端常量时间验证凭据，生成不可预测会话 ID 与 CSRF token；只将会话 ID 写入 HttpOnly Cookie，CSRF token 经响应体交给前端内存态。
4. 路由 guard 在认证查询完成前不挂载受保护页面；登录成功只导航到校验通过的站内目标。
5. 管理变更和退出请求携带 Cookie 与 CSRF header。服务端先验证会话有效期，再验证 CSRF token，最后进入 handler。
6. 24 小时到期、退出或重启后会话失效；到期页面保留不含凭据的路由/筛选上下文并回到登录页。

## API 行为约定

- `POST /api/auth/login`：JSON `{username,password}`；成功设置 Cookie 并返回认证状态、CSRF token；失败统一 401 与通用提示。禁止在响应、日志中回显用户名或密码。
- `GET /api/auth/session`：有效会话返回 `{authenticated:true, csrfToken:...}`；无效/缺失/过期会话返回 `{authenticated:false}`，并清除过期 Cookie。
- `POST /api/auth/logout`：需要有效会话及 CSRF token；成功撤销当前 session、清除 Cookie，返回 204。
- 管理 API：受认证中间件保护。未来新增的管理路由必须显式加入保护组；健康端点公开。

## 兼容性、安全与运维

- 配置增加必填凭据意味着未配置的新部署会在启动阶段失败，这是 fail-closed 的预期行为；Compose 示例须清楚给出注入方式。
- ENV 对单个字段覆盖 YAML；空 ENV 不回退。用户名校验按 trim 后非空，用户名比较可按配置原文精确匹配；密码保留空格作为密码内容，但 trim 后全空白拒绝。
- 密码以明文存在于部署配置，运维需保护环境变量/配置文件访问权限。应用不得记录配置对象、请求体、Cookie、CSRF token 或密码。
- Cookie `Secure` 默认关闭以兼容 HTTP 回环部署，通过 HTTPS 服务时需显式配置为 true；禁止从任意转发头自动判断。
- SPA 深链由现有 web UI fallback 提供。登录页和 guard 不应把完整 URL 或任何凭据序列化到持久化存储。
- 会话纯内存且单实例；多实例部署会导致会话随机失效，超出本任务支持范围。
- 退出只撤销当前 Cookie 对应会话；重启自然清空全部会话；配置修改需重启后生效。

## 验证与回滚

- 覆盖配置优先级、显式空 ENV、缺失/空白配置启动失败。
- 覆盖登录成功/错误用户名/错误密码、会话查询、退出、24 小时过期、重启失效语义，以及管理 API 未认证/CSRF 缺失或错误拒绝。
- 检查日志不含秘密，浏览器持久化无认证秘密，Cookie 属性和站内跳转校验符合约定。
- OpenAPI 变更后运行生成器并核对生成文件；运行 `make check`、`make smoke` 和前端构建。
- 回滚按文件粒度撤销认证 handler、中间件、前端 guard、API 契约和配置文档；不涉及数据库迁移或用户数据回滚。部署回滚需同时恢复旧配置，旧版本不识别新 YAML 字段时不能复用包含新字段的配置文件。
