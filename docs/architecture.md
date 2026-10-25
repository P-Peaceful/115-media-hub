# 115-media-hub 初始化架构

依据已确认的前后端架构清单，采用模块化单体：Go/Gin/GORM/SQLite/goose 后端与 React 19/Vite/TypeScript/HeroUI v3/Tailwind v4 前端。

- `backend/` 独立 Go module，`frontend/` 独立 pnpm 项目，`api/openapi.yaml` 是共享契约。
- HTTP → Service → Domain；Repository 负责持久化，Adapter 对接外部能力。Gin Context 仅存在于 HTTP 层。
- 生产环境先构建前端，再复制到 `backend/internal/webui/dist`，最后通过 go:embed 嵌入应用。
- 开发环境由 Vite 代理 API 和健康检查到 Go。
- SQLite 启用 WAL、外键、busy timeout，单写连接；结构变更通过 goose SQL，禁止 AutoMigrate。
- 基础架构交付配置、日志、数据库与迁移、健康检查、静态资源托管、前端路由/主题/请求基础、代码生成和 Docker 链路。
- 管理员认证已实现：启动配置一组用户名与密码，服务端内存会话固定有效 24 小时，管理 API 通过 Cookie 和 CSRF 中间件保护，前端提供登录及受保护工作台框架。健康检查公开；详情见[开发说明](development.md)。
- 媒体库、任务执行器、SSE 业务事件、115/媒体服务接入等仍未实现。不得用空成功响应冒充功能。
- `/data`、`/storage/strm`、`/storage/quarantine`、`/cache`、`/logs`、`/backups` 为容器运行目录，不提交运行数据。

来源：
- https://app.notion.com/p/401138389edc811fabfcefbf05f26845
- https://app.notion.com/p/401138389edc814fb54be80c599600ef
