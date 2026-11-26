# 日志模块开发（运行日志）

## Goal

设计和实现 115 Media Hub 全栈「运行日志」模块，对照用户确认的设计图与交互决策，在侧栏「任务与维护 → 运行日志」提供按时间、级别、模块、任务、关键词筛选的系统运行日志视图，支持 SSE 实时跟随自动滚动、错误高亮、流式日志导出及带安全审计保护的日志清理能力。

## Background & Confirmed Facts

- **运行时日志现状**（[logging.go](file:///home/wenzhengcheng/github/115-media-hub/backend/internal/app/logging.go)、[config.go](file:///home/wenzhengcheng/github/115-media-hub/backend/internal/app/config.go)）：
  - 后端统一使用 `*slog.Logger` + `slog.NewJSONHandler`，输出标准 JSON 日志行。
  - 配置了 `MEDIAHUB_LOG_FILE` 时通过 `io.MultiWriter` 将同一份 JSON 日志同时写入 `stdout` 和 `lumberjack.v2` 轮转文件（10 MiB / 3 份备份 / 7 天）；本地开发默认为空（仅写 `stdout`）。
  - 当前后端已发射的 `module` 包括：`app`、`http`、`health`、`auth`、`provider_115`、`library`、`jobs`、`strm`、`logging`。
- **任务日志现状**（[00005_task_center.sql](file:///home/wenzhengcheng/github/115-media-hub/backend/migrations/00005_task_center.sql)、[task_jobs.go](file:///home/wenzhengcheng/github/115-media-hub/backend/internal/repository/sqlite/task_jobs.go)）：
  - `task_logs` 表记录任务执行过程中的中文业务日志，目前仅在任务详情页展示。
- **安全审计现状**（[00002_administrator_auth.sql](file:///home/wenzhengcheng/github/115-media-hub/backend/migrations/00002_administrator_auth.sql)）：
  - `audit_records` 表已由管理员认证、安全设置、115 授权/解绑、媒体库删除及定时计划删除事务写入，与运行日志相互独立。
- **前端入口占位**（[app-shell.tsx](file:///home/wenzhengcheng/github/115-media-hub/frontend/src/app/app-shell.tsx#L58)）：
  - 侧栏「任务与维护」分组下已预留「运行日志」导航项（当前禁用，无路由）。

## Key Product Decisions

- **D1 运行日志数据源与持久化**：保留现有 `stdout` + `lumberjack.v2` 轮转文件；新增 SQLite `runtime_logs` 表并通过自定义 `slog.Handler` 异步批量采集系统与业务运行日志（同时汇聚任务执行日志的 `job_id` 关联），带自动保留上限（30 天 / 50,000 条）与有界 SSE 实时流，确保本地开发与 Docker 环境体验一致。
- **D2 页面结构与审计边界**：严格按设计图交付「运行日志」单页，不新增顶部审计标签页；后端将「清理日志」等破坏性操作原子写入 `audit_records` 表，且清理运行日志永不删除 `audit_records` 与任务中心 `task_jobs` / `task_logs`。
- **D3 「清理日志」交互与范围**：点击「清理日志」弹出确认弹窗，提供清理范围选择——**清理 7 天前**（默认选中）、**清理 30 天前**、**清空全部运行日志**；明确提示仅清理系统运行日志，确认后返回删除条数并原子记录 `clear_runtime_logs` 审计事件。
- **D4 列表排序、实时跟随与导出**：
  - 日志列表按时间倒序展示（最新记录位于顶部，与设计图一致）。
  - 开启「自动滚动」时（首页默认开启），订阅有界 SSE 实时流，将匹配当前筛选条件的新日志插入顶部并自动定位到顶部最新记录；关闭时断开实时流，保持当前快照，支持手动「刷新」。
  - 「下载日志 ▾」下拉提供当前筛选条件下的 2 种流式导出（单次上限 10,000 条）：**纯文本 `.log`** 与 **结构化 `.jsonl`**。

## Requirements

### R1 后端运行日志采集、脱敏与留存
- **R1.1 统一采集且不阻塞业务**：在保留 `stdout` 与 `lumberjack.v2` 输出不变的前提下，通过包装 `slog.Handler` 将系统与业务运行日志写入有界内存队列，由后台批处理协程写入 SQLite `runtime_logs` 表，进程优雅关闭时在关闭 SQLite 前刷盘完毕。队列满时丢弃新采集事件并做限频诊断，绝不阻塞调用方或引发 SQLite 单连接死锁。
- **R1.2 噪声过滤与防递归**：默认不将 `/health/*` 探针、`GET /api/v1/logs*` 自访问请求、常规 2xx/3xx HTTP 访问日志及 `logging` 模块内部写入/推流诊断写入 `runtime_logs`，避免打开日志页产生递归刷屏。
- **R1.3 脱敏与中文可读内容**：对所有入库及推送的日志消息和允许列表内的结构化属性执行脱敏，严禁保存或透出密码、哈希、Cookie（`UID`/`CID`/`SEID`/`KID`）、Session/CSRF Token、Authorization 头及临时播放链接。对已有英文结构化 `slog` 事件映射为清晰中文可读摘要，同时在 `details_json` 中保留脱敏结构化字段；任务执行日志通过 `job_id` 与对应模块统一呈现。
- **R1.4 有界留存**：自动按留存策略（默认保留最近 30 天且不超过 50,000 条）分批裁剪过期运行日志，避免 SQLite 体积无限增长。

### R2 OpenAPI 契约与后端管理接口
所有接口位于 `/api/v1/logs*`，受现有管理员会话与写请求 CSRF/Same-Origin 边界保护，并先在 `api/openapi.yaml` 定义再执行 `make generate`：
- **R2.1 历史日志分页查询 `GET /api/v1/logs`**：支持按 `range`（`1h`/`24h`/`7d`/`30d`/`all`，默认 `24h`）、`level`（`debug`/`info`/`warn`/`error`）、`module`、`job_id`、`q`（关键词，最长 128 字符，安全转义匹配消息/操作/错误码/任务 ID/请求 ID）、`limit`（1–200，默认 50）、`cursor`（不透明游标）进行倒序（`occurred_at DESC, id DESC`）分页查询，返回 `{items, next_cursor}`。
- **R2.2 实时日志流 `GET /api/v1/logs/stream`**：返回 `text/event-stream`，支持按 `level`/`module`/`job_id`/`q` 服务端过滤，推送具名事件 `log.created`（携带 `RuntimeLogEntry` JSON）及 `log.cleared`。沿用现有 SSE 安全规范：连接内递增事件 ID、15 秒心跳并在发送前复核管理员会话有效性、单帧 5 秒写超时、有界订阅队列（慢客户端不阻塞业务）、最大并发订阅数限制（超限返回 `503`）。
- **R2.3 日志流式导出 `GET /api/v1/logs/export`**：支持 `format=log`（可读纯文本行）与 `format=jsonl`（脱敏 JSON 行）以及与查询一致的筛选参数；按时间倒序流式输出最多 10,000 条记录，设置 `Content-Disposition` 附件文件名与 `X-Export-Truncated` 响应头，并在完成/失败时记录一条 `module="logging", operation="export"` 终端日志。
- **R2.4 日志清理 `POST /api/v1/logs/clear`**：需校验 `X-CSRF-Token` 与同源头，接收 `{scope: "before_7d" | "before_30d" | "all"}`；在单个 SQLite 事务内删除匹配的 `runtime_logs` 记录并原子写入 `audit_records`（`action="clear_runtime_logs"`），审计写入失败时整笔回滚并返回 `503`；成功返回 `{deleted_count, scope}`，记录一条 `module="logging", operation="clear"` 终端日志，并通知活跃 SSE 订阅者。

### R3 前端「运行日志」页面与导航接入
- **R3.1 导航与路由**：启用侧栏「任务与维护 → 运行日志」链接至 `/logs`，并在顶部面包屑正确归属「任务与维护 / 运行日志」。
- **R3.2 页面结构与设计还原**：使用 HeroUI v3 组件与语义主题令牌（兼容浅色/深色主题）还原设计图：
  - 顶部 `PageBanner`：标题「运行日志」，描述「查看系统运行日志，支持按时间、级别、模块、关键词筛选，并可下载与清理日志。」
  - 第一行筛选栏：时间范围（默认「最近 24 小时」）、日志级别（默认「全部」）、模块（默认「全部」）、任务（默认「全部」，来源于任务列表并兼容 URL 指定的 `job_id`）、关键词搜索框（占位「搜索关键词（文件名、路径、错误信息）」，防抖并同步到 URL 查询参数）。
  - 第二行操作栏：`自动滚动` 开关、`高亮错误` 复选框（默认勾选）、`刷新` 按钮、`下载日志 ▾` 下拉菜单（当前筛选 `.log` / `.jsonl`）、红色 `清理日志` 按钮。
  - 五列表格：`时间`（本地时区格式化、等宽数字）、`级别`（柔和色 Chip：`INFO`/`WARN`/`ERROR`/`DEBUG`）、`模块`（中文名称）、`任务`（有 `job_id` 时展示可点击跳转 `/tasks/:id` 的任务链接，无则显示 `-`）、`日志内容`（清晰可读消息；开启「高亮错误」时 `ERROR` 整行浅红高亮；支持点击或展开查看关联 `request_id`/`error_code` 等脱敏详情）。
- **R3.3 实时流、分页与交互状态**：
  - 位于第 1 页且「自动滚动」开启时建立 SSE 连接，新日志插入顶部（有前端内存条数上限避免 DOM 无限增长）并自动滚回顶部；切换筛选条件自动重连，翻到非第 1 页或关闭「自动滚动」时停止实时插入；登出或离开页面时释放连接。
  - 清理弹窗支持选择 `清理 7 天前`（默认）/ `清理 30 天前` / `清空全部运行日志`，展示不影响审计与任务记录的提示，确认后刷新列表并反馈删除条数。
  - 完整覆盖加载中、空状态、接口错误重试、导出截断提示及可访问性（键盘可达、语义标签）。

## Out of Scope

- 安全审计记录（`audit_records`）的独立查询接口与前端展示标签页（保留供后续独立任务扩展）。
- 在线动态修改后端 `MEDIAHUB_LOG_LEVEL` 或 `lumberjack` 轮转配置。
- 直接删除或截断宿主机/容器上的 `MEDIAHUB_LOG_FILE` 轮转文件。
- 尚未实现的业务模块（如文件回传、海报墙）的真实业务执行逻辑（仅在模块映射中预留标签，不伪造业务数据）。

## Acceptance Criteria

- [x] **AC1 契约一致**：`api/openapi.yaml` 定义了 `/api/v1/logs`、`/api/v1/logs/stream`、`/api/v1/logs/export`、`/api/v1/logs/clear` 及全部请求/响应模型；`make generate` 与 `make check-generated` 零差异通过。
- [x] **AC2 持久化与防阻塞**：启动应用后，业务与系统 `slog` 事件及任务执行日志均自动写入 SQLite `runtime_logs` 表，进程重启后历史日志仍可查；原有 `stdout` 与 `lumberjack` 文件日志行为无回归；队列满或入库失败时不阻塞业务且不发生递归刷屏。
- [x] **AC3 脱敏与过滤**：包含 Cookie、Authorization、Token、临时下载链接的日志不会以明文落入 `runtime_logs`、SSE 推送或导出文件；`/health/*` 及日志页自身常规请求不会刷屏 `runtime_logs`。
- [x] **AC4 查询与导出**：`GET /api/v1/logs` 按时间范围、级别、模块、任务、关键词组合筛选及游标倒序分页正确；`GET /api/v1/logs/export` 正确流式输出 `.log` 与 `.jsonl`（超限时置 `X-Export-Truncated: true`）并产出一条 `logging/export` 终端日志。
- [x] **AC5 实时流安全**：`GET /api/v1/logs/stream` 按筛选条件推送新日志，慢消费者不阻塞业务写入，会话注销/过期后连接及时终止，前端卸载或关闭「自动滚动」后正确关闭 `EventSource`。
- [x] **AC6 清理与原子审计**：`POST /api/v1/logs/clear` 校验 CSRF 与范围参数，按 `before_7d`/`before_30d`/`all` 正确删除 `runtime_logs` 并在同一事务中写入 `audit_records`；审计表写入失败时回滚清理操作；清理后 `audit_records`、`task_jobs` 与 `task_logs` 均完好无损。
- [x] **AC7 前端还原与交互**：侧栏「运行日志」可正常进入 `/logs`，页面布局、筛选栏、工具栏、表格样式及「高亮错误」「自动滚动」「下载日志」「清理日志弹窗」在浅色与深色主题下表现一致且响应式正常。
- [x] **AC8 质量门禁**：前后端单元测试、竞态检查（`go test -race`）、`make check`、以及针对 `/logs` 的 Playwright E2E 测试全部通过；手写 Go 文件满足中文注释规范。
