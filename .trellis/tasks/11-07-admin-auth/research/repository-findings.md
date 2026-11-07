# 认证实现的仓库现状

## 现有基础

- `frontend/src/app/app.tsx` 组装一个 QueryClient 和 BrowserRouter；当前 `/` 渲染 `HealthPage`，通配路由渲染未找到页。
- `frontend/src/styles/app.css` 定义共享亮色/系统暗色 token 和减少动态效果规则。当前没有认证资源或布局。
- `frontend/package.json` 锁定 HeroUI React/styles 3.2.7、React 19.3.1、React Router 8.5.1、Query 5.106.0 和 Lucide 1.68.0。控件开发前查阅 v3 实际文档。
- `backend/internal/app/app.go` 在调用 `transport.NewRouter` 前打开 SQLite；新服务应在此构造，而不是放在 handler 中。
- `backend/internal/repository/sqlite/database.go` 打开 GORM 后目前只保留 `*sql.DB`。应明确扩展仓储归属以支持功能查询，不能把 GORM 泄漏到 HTTP 层。
- `backend/internal/http/router.go` 负责公开 live/ready 健康处理器、安全错误封装/请求 ID 日志和按命名空间处理 SPA 回退。当前没有认证路由或中间件。
- `backend/migrations/00001_application.sql` 只建立 application_id。新增迁移；现有/全新数据库升级后都应进入初始化。
- `api/openapi.yaml` 目前只有健康检查 DTO 和四种通用错误码。定义认证契约后重新生成两端产物。
- `backend/go.mod` 已间接锁定 `golang.org/x/crypto v0.58.0`。复用/提升现有 bcrypt 依赖，避免随意选新哈希库。TTY 恢复可能需要单独锁定终端输入依赖。
- `backend/cmd/mediahub/main.go` 目前没有 CLI 解析。新增维护子命令时保持普通启动兼容。

## 验证与集成风险

- `frontend/playwright.config.ts` 使用 fullyParallel=true 运行 desktop/mobile-dark 项目；`scripts/test-integration.py` 整次运行只启动一个真实后端/数据库。有状态的初始化/登录测试必须隔离，或明确串行共享后端状态。
- 根目录 `Makefile` 提供代码生成、完整本地检查、模拟 E2E、真实后端集成、嵌入式构建/smoke 和 Docker 构建/smoke。实际结果写入任务验证文档。
- 保护应用首页时，既有健康组件/E2E 以及后端 router/app 测试应保留或适配；`/health/live` 和 `/health/ready` 仍公开。

## 视觉与产品覆盖项

截图包含两个独立画板，不是一个合并界面。用户要求优先：相关页面完全移除 115 授权，并移除整个顶部状态栏/标题栏。保留登录主内容品牌标识和紫色/文件夹视觉风格。密码恢复走服务器本地命令；不增加邮件配置或公开重置端点。最终规划评审还确认了有效期/路径默认值和固定过期语义。

## 组件文档核实

2026-11-07，HeroUI MCP `list_components` 确认可用目录为 v3.2.7，与项目锁定依赖一致。相关可用组件有 Form、TextField、Input、InputGroup、Label、FieldError、Button、Card、Checkbox、Select、ListBox、Modal、Link、Alert 和 Spinner。此结果只确认组件存在；编码前须对实际选用组件调用 `get_component_docs`，不得从名称猜测 v3 API。

用户最新质量要求是明确验收条件：适用前端组件优先使用 HeroUI；前后端不使用已弃用方法；高保真还原调整后设计；代码可读且职责聚焦。评审和证据除功能测试外也应覆盖这些要求。

用户最新要求将默认服务器输出目录改为 `/app/media`。Docker 部署的外部卷容器目标位于 `/app/` 下；本地开发数据库仍应保存在项目 `backend/data/`。部署文档需明确区分容器路径和本地项目路径。

## 已确认的开发代理问题

首轮真实浏览器初始化返回 403 CSRF，尽管 Cookie/证明及浏览器 Origin/Host 看似匹配。脱敏后的上游回显显示 Host 被改写为后端目标，而 Origin 仍为浏览器源。安装的 Vite 8.3.5 `dist/node/chunks/node.js` 会把字符串代理配置扩展为 `{target: opts, changeOrigin: true}`。在 `/api` 和 `/health` 上显式设置 `{target, changeOrigin: false}` 可在不信任转发头/放宽后端检查的前提下修复所属边界。之后真实桌面端 4/4、暗色移动端 4/4 流程均通过。
