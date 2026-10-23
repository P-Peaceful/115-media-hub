# 开发说明

## 初始化边界

已有：前后端工程配置、健康检查、SQLite 连接与 goose 迁移、JSON 日志与轮转、信号退出、静态资源与 SPA 回退、主题切换、React Router、TanStack Query Provider、Zustand 界面偏好、请求基础设施、OpenAPI 生成、Docker 联合构建。

未实现：登录/Session/CSRF 服务端校验、115 授权和浏览、媒体库、扫描、STRM、播放解析、任务调度/持久化执行/SSE 事件、海报墙、元数据回传、审计、备份恢复。对应目录只有 `.gitkeep`，无业务数据表和成功占位接口。cron 等业务阶段依赖届时再引入；Motion、TanStack Table/Virtual 按需引入。

## 配置

优先级：默认值 < `-config` 指定的 YAML < 环境变量。

| 环境变量 | 默认值 |
| --- | --- |
| `MEDIA_HUB_HTTP_ADDR` | `127.0.0.1:8080` |
| `MEDIA_HUB_DATABASE_PATH` | `./data/115-media-hub.db` |
| `MEDIA_HUB_LOG_DIR` | `./logs` |
| `MEDIA_HUB_SHUTDOWN_TIMEOUT` | `10s` |

相对路径以进程工作目录为基准。根目录 `.env.example` 是配置说明；Go 不自动加载 `.env`。Vite 代理目标通过启动环境变量 `MEDIA_HUB_DEV_API_URL` 指定，例如 `MEDIA_HUB_DEV_API_URL=http://127.0.0.1:9090 make dev-frontend`。

YAML 使用严格字段校验，未知字段、非法端口、空数据库路径和非正退出超时会阻止启动。

## 数据库和日志

数据库必须位于本地磁盘，启用 WAL、foreign_keys、5000ms busy_timeout，并将连接池限制为一个连接。只允许 goose 版本化 SQL 修改结构，不使用 GORM AutoMigrate。当前基线迁移仅执行 SELECT 1，数据库只有 goose 版本管理表。

数据库文件以 0600 创建，目录以 0700 创建。日志写入标准输出与 `app.log`，轮转阈值 20 MiB、最多 5 个备份、保留 14 天、压缩旧文件。HTTP 日志仅记录方法、匹配路由、状态和耗时，不输出原始 URL、请求体、Cookie 或凭据。未来业务日志仍须遵循脱敏约定。

## 前端

HeroUI v3 使用 `@heroui/react` 与 `@heroui/styles`，不需要 v2 的 Provider。CSS 导入顺序为 Tailwind → HeroUI → 应用变量。主题仅存储在 Zustand 偏好中；服务端数据归 TanStack Query；导航目录、筛选、分页应使用 URL。

请求封装只接受同源绝对路径，使用 same-origin Cookie；变更请求可显式传入 CSRF token。此能力不是完整认证方案，需后续会话服务提供 token 和后端校验。未实现的 SSE 连接与会话逻辑暂不添加。

## 契约与构建

修改 `api/openapi.yaml` 后运行 `make generate`，提交契约及两端生成文件。Go 健康检查实现通过生成的 ServerInterface 注册，避免手工路由与契约漂移。

`make build` 将 `frontend/dist` 复制到 `backend/internal/webui/dist`，以 `embedui` build tag 构建生产程序。默认 Go 开发构建无此前端资源依赖。产物目录不提交。

根目录 Makefile 是跨工程入口，不需要 Go workspace 或 pnpm workspace。Go module 名称暂使用 `github.com/wenzhengcheng/115-media-hub/backend`；若实际远端地址不同，后续统一替换 module 与导入路径。

## 验证

`make check` 检查格式、Go vet、Go 测试、前端类型、ESLint、peer 依赖及 Vitest。`make smoke` 使用临时目录启动真实二进制，检查健康接口、嵌入首页、SPA 深链、API/静态资源 404、数据库无业务表和 SIGTERM 退出，测试结束后清理临时数据。

Docker Compose 默认仅绑定宿主机回环地址。运行用户为 `mediahub`，UID/GID 为 10001；改用 bind mount 时应确保目录可写，隔离区必须在媒体扫描范围之外。恢复和清理业务尚未实现。

本地 Makefile 使用 `-buildvcs=false`，支持无可用 Git 元数据的源码目录。项目环境变量前缀为 `MEDIA_HUB_`，默认数据库文件名为 `115-media-hub.db`。
