# 日志模块技术设计（Design）

## 1. Architecture & Layer Boundaries

遵循后端单进程模块化单体分层（`domain` → `service` → `repository/sqlite` / `http`）与前端特性目录边界：

```text
slog callers (app / auth / provider115 / library / jobs / strm / http)
    │
    ▼
[CollectorHandler] (wraps existing stdout + lumberjack JSONHandler)
    ├─► next.Handle() ──────────────────────► stdout + lumberjack (unchanged)
    └─► filter + redact + format ──► [logs.Service]
                                         ├─► bounded SSE subscribers (non-blocking)
                                         └─► bounded ingest queue (cap 2048)
                                                 │ (background batch flusher)
                                                 ▼
                                          SQLite: runtime_logs (with retention prune)

HTTP Admin Client (/logs)
    ├─ GET  /api/v1/logs         ──► logsHandler ──► logs.Service ──► sqlite.ListRuntimeLogs
    ├─ GET  /api/v1/logs/stream  ──► logsHandler ──► logs.Service.Subscribe (SSE)
    ├─ GET  /api/v1/logs/export  ──► logsHandler ──► logs.Service.Export (streaming)
    └─ POST /api/v1/logs/clear   ──► logsHandler ──► logs.Service.Clear ──► sqlite tx (DELETE + INSERT audit_records)
```

### 包职责划分
- `backend/internal/domain/logs/`：纯领域模型与常量（`Entry`, `Level`, `Module`, `Filter`, `ClearScope`, `Page`, `ExportFormat`, 敏感字段脱敏与消息格式化纯函数，无 DB/HTTP 依赖）。
- `backend/internal/service/logs/`：
  - `Service`：管理有界入库队列、后台批刷与保留裁剪协程、SSE 订阅广播器、查询/导出/清理用例与 `Close(ctx)` 优雅刷盘。
  - `CollectorHandler`：实现 `slog.Handler`，组合现有 JSON handler，不阻断原日志输出。
- `backend/internal/repository/sqlite/runtime_logs.go`：SQLite `runtime_logs` 批量插入、游标分页查询、流式批次查询、定期裁剪，以及「清理运行日志 + 写入 `audit_records`」原子事务。
- `backend/migrations/00009_runtime_logs.sql`：新建 `runtime_logs` 表与组合索引。
- `backend/internal/http/logs.go` + `logs_stream.go`：Gin 路由注册、参数校验、SSE 连接保活/会话复核、流式导出响应与错误映射。
- `frontend/src/features/runtime-logs/`：`api.ts`、`queries.ts`、`log-stream.ts`、`log-labels.ts`、`runtime-logs-page.tsx`、`clear-logs-dialog.tsx`、`runtime-logs-table.tsx` 等。

## 2. Database Schema & Indexing

迁移文件：`backend/migrations/00009_runtime_logs.sql`

```sql
-- +goose Up
CREATE TABLE runtime_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at DATETIME NOT NULL,
    level TEXT NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
    module TEXT NOT NULL,
    operation TEXT NOT NULL DEFAULT '',
    job_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_runtime_logs_time_id ON runtime_logs (occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_level_time ON runtime_logs (level, occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_module_time ON runtime_logs (module, occurred_at DESC, id DESC);
CREATE INDEX idx_runtime_logs_job_time ON runtime_logs (job_id, occurred_at DESC, id DESC) WHERE job_id != '';

-- +goose Down
DROP TABLE IF EXISTS runtime_logs;
```

### 游标分页契约
- 排序固定为 `occurred_at DESC, id DESC`。
- 不透明 `cursor` 使用 Base64URL 编码 JSON：`{"t":"2026-11-27T06:00:00.123456789Z","id":12345}`。
- 查询条件：`WHERE (occurred_at < ? OR (occurred_at = ? AND id < ?))`；多取 1 条（`limit + 1`）以确定 `next_cursor` 是否为 `null`。
- 无效或损坏的 `cursor` 返回 `400 validation_failed`。

### 保留上限与裁剪策略
- 默认保留：**30 天** 且最多 **50,000 条**。
- 后台批刷协程每 10 分钟（或累计写入 1,000 条后）执行一次有界裁剪：
  1. `DELETE FROM runtime_logs WHERE id IN (SELECT id FROM runtime_logs WHERE occurred_at < ? LIMIT 1000)`；
  2. 若总行数超过 50,000，按 `id ASC` 删除超出部分（每次最多 1,000 行）。
- 分批删除确保单连接 SQLite 事务极短，不阻塞并发业务读写。

## 3. Collector, Noise Filtering, Redaction & Module Mapping

### 3.1 `app.New` 装配与关闭顺序（避免单连接死锁与丢失尾部日志）
- `backend/cmd/mediahub/main.go` 保持不变，`app.NewLogger` 签名不变（CLI 维护命令无需启动异步日志协程）。
- 在 `backend/internal/app/app.go` 中：
  1. `sqlite.Open` 成功后立刻构造 `runtimeLogs := logs.New(database, logger, logs.Options{})`；
  2. 通过 `wrappedLogger := runtimeLogs.WrapLogger(logger)` 得到带 `CollectorHandler` 的 `*slog.Logger`；
  3. 将 `wrappedLogger` 注入 `auth`、`library`、`provider115`、`strm`、`jobs`、`transport.NewRouter` 和 `App.logger`；
  4. `jobs.Service` 在调用 `InsertJobLog` / 生命周期写入任务日志时，通过可选回调或 `wrappedLogger`（带 `job_id`、映射后的业务模块与已脱敏中文内容）同步进入 `runtimeLogs`，同时对重复的英文状态切换日志做好去重；
  5. `App.Serve` 优雅关闭顺序：`server.Shutdown` + `jobs.Stop` → 记录 `"HTTP server stopped"` → `a.logs.Close(shutdownCtx)`（排空队列并刷盘） → `App.Close()` 关闭 SQLite。

### 3.2 噪声过滤与防递归规则
- **跳过入库与 SSE 广播的记录**：
  - `module == "health"`（所有健康探针）。
  - `module == "http"` 且 `level < WARN`（常规 2xx/3xx HTTP 请求，包括日志页自身的 `/api/v1/logs*` 请求；但保留 `WARN`/`ERROR`/panic 等异常请求）。
  - `module == "logging"` 且 `operation` 属于内部采集/刷盘/推流/裁剪（`write`, `ingest`, `flush`, `prune`, `stream_deliver`）；保留面向管理员的 `export` 和 `clear` 结果日志。
- **队列满保护**：`ingestCh` 容量 2048，非阻塞 `select` 投递；满时累加 `dropped` 原子计数，最多每 30 秒向原始底层 `baseLogger` 输出一条 `module="logging", operation="ingest", error_code="log_queue_full"` 警告（不经 `CollectorHandler`，彻底杜绝递归）。

### 3.3 模块规范化映射
| 规范 `module` 值 | 前端展示名称 | 来源映射 |
| --- | --- | --- |
| `sync` | 同步 | 同步任务/同步事件 |
| `scan` | 扫描 | 扫描任务/115 递归扫描事件 |
| `strm` | STRM 生成 | `module="strm"` 及 STRM 任务 |
| `upload` | 回传 | 预留给文件回传/元数据上传模块 |
| `jobs` | 任务调度 | `module="jobs"`（调度触发、排队、暂停、取消、重试） |
| `library` | 媒体库 | `module="library"` |
| `provider_115` | 115 网盘 | `module="provider_115"`（授权、扫码、限流、目录浏览） |
| `auth` | 认证与安全 | `module="auth"`（登录、改密、会话设置、CSRF 拒绝） |
| `system` | 系统 | `module="app"`, `module="http"`(异常), `module="logging"`(导出/清理) 及其他未知模块 |

### 3.4 脱敏与中文可读消息合成
- **脱敏白名单与正则净化**：
  - 丢弃任何 key 包含 `password`、`secret`、`token`、`cookie`、`authorization`、`credential`、`hash`、`url`（临时下载链接）的属性；
  - 对 `message` 与保留的字符串属性进行模式净化，替换 `Cookie:`、`Authorization:`、`UID=`/`CID=`/`SEID=`/`KID=`、`Bearer ` 及含查询签名的外部链接为 `[REDACTED]`；
  - 截断单条 `message` 最大 1,024 字符、`details_json` 最大 2,048 字节。
- **中文可读消息**：
  - 若源消息已包含中文（如任务日志或中文业务事件），脱敏后直接使用；
  - 对现有后端英文 `slog` 消息（如 `database initialized`、`HTTP server started`、`authentication operation completed`、`115 operation completed`、`library operation completed`、`task execution state changed`、`STRM operation finished`、`STRM full sync preview finished`、`request panic` 等），按 `(module, operation, outcome, error_code)` 格式化为自然中文描述，未知消息安全回退为脱敏后的原始 `msg`。

## 4. HTTP & SSE Contract (`api/openapi.yaml`)

### 4.1 端点一览
| 方法与路径 | Operation ID | 认证与 CSRF | 说明 |
| --- | --- | --- | --- |
| `GET /api/v1/logs` | `listRuntimeLogs` | Admin Session | 多维筛选 + 倒序游标分页 |
| `GET /api/v1/logs/stream` | `streamRuntimeLogs` | Admin Session | SSE 实时运行日志流 |
| `GET /api/v1/logs/export` | `exportRuntimeLogs` | Admin Session | 流式导出 `.log` / `.jsonl`（上限 10,000 条） |
| `POST /api/v1/logs/clear` | `clearRuntimeLogs` | Admin Session + CSRF + Same-Origin | 按范围清理运行日志并写审计 |

### 4.2 核心 DTO（snake_case）
- `RuntimeLogEntry`：
  - `id`: `string`（数据库自增 ID 的字符串形式，遵循契约“HTTP 边界 ID 一律为不透明字符串”）
  - `occurred_at`: `string` (`date-time`, RFC 3339 UTC)
  - `level`: `"debug" | "info" | "warn" | "error"`
  - `module`: `"sync" | "scan" | "strm" | "upload" | "jobs" | "library" | "provider_115" | "auth" | "system"`
  - `operation`: `string`
  - `job_id`: `string | null`
  - `request_id`: `string | null`
  - `error_code`: `string | null`
  - `message`: `string`
  - `details`: `Record<string, string | number | boolean>`（可选或空对象）
- `RuntimeLogPage`：`{ items: RuntimeLogEntry[], next_cursor: string | null }`
- `ClearRuntimeLogsRequest`：`{ scope: "before_7d" | "before_30d" | "all" }`
- `ClearRuntimeLogsResponse`：`{ deleted_count: int64, scope: "before_7d" | "before_30d" | "all" }`

### 4.3 SSE 实时流设计（`backend/internal/http/logs_stream.go`）
- 复用 `task_events.go` 经验证的安全模式：
  - 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-store, no-transform`、`X-Accel-Buffering: no`。
  - 订阅前先写入 `: connected\n\n` 并刷新。
  - 每个订阅者独立缓冲 channel（容量 128），广播时非阻塞投递，慢客户端丢弃而非阻塞发布者；最多允许 16 个并发流订阅者，超限返回 `503 service_unavailable`。
  - 每次发送事件或 15 秒 `: keep-alive\n\n` 心跳前，调用 `h.auth.Authenticate(ctx, token)` 复核管理员会话，会话失效立即退出。
  - 每帧设置 5 秒写超时（`http.ResponseController.SetWriteDeadline`）并清除。
  - **关键点**：`logs.Service` 在广播给 SSE 订阅者前先分配临时/持久 ID？
    - 为了避免 SSE 推送的 `id` 与刷新后从 SQLite 读到的 `id` 重复导致前端 key 冲突，SSE 广播放在 **后台批刷 `INSERT` 成功获得真实 `runtime_logs.id` 之后**（批刷周期 200ms，肉眼完全实时，且保证 SSE 条目与数据库 ID 100% 一致、绝不出现“推了但没入库”或重复行！）。

### 4.4 流式导出设计
- `GET /api/v1/logs/export?format=log|jsonl&range=...&level=...&module=...&job_id=...&q=...`
- 按每批 500 条从 SQLite 游标迭代，最多输出 10,000 条（查 10,001 条以判断是否截断）。
- 设置响应头：
  - `Content-Type: text/plain; charset=utf-8`（`.log`）或 `application/x-ndjson; charset=utf-8`（`.jsonl`）
  - `Content-Disposition: attachment; filename="mediahub-logs-YYYYMMDD-HHMMSS.<ext>"`
  - `X-Export-Truncated: true|false`
  - `Cache-Control: no-store`
- **注意 HTTP `WriteTimeout`**：服务器当前 `WriteTimeout: 75s`，导出上限 10,000 条本地 SQLite 读取约数毫秒至百毫秒级，并用 `ResponseController.Flush()` 边读边写；完成时记录 1 条 `module="logging", operation="export"` 日志。

## 5. Frontend Architecture (`frontend/src/features/runtime-logs/`)

### 5.1 URL 与状态所有权
- **URL Search Params**（单一事实源，刷新/分享链接保留）：`range`（默认 `24h`）、`level`、`module`、`job_id`、`q`、`cursor`。
- **TanStack Query**：
  - `runtimeLogKeys.list(filters)` → `GET /api/v1/logs`
  - `taskKeys`（复用 `listTasks` 拉取最近 50 个任务填充「任务」筛选下拉框）
  - `useClearRuntimeLogs()` mutation → `POST /api/v1/logs/clear`（携带 `useAuth().csrfToken`，成功后失效 `runtimeLogKeys.all` 并弹出结果提示）
- **组件局部状态**：
  - `autoScroll`（boolean，默认 `true`）
  - `highlightErrors`（boolean，默认 `true`）
  - `liveItems`（通过 `EventSource("/api/v1/logs/stream?...")` 收到的新日志，按 `id` 与查询结果去重并插入顶部，上限 200 条）
  - `streamStatus`（`"idle" | "connecting" | "connected" | "reconnecting"`）
  - `clearDialogOpen`（boolean）与 `clearScope`（`"before_7d" | "before_30d" | "all"`）
  - 表格容器 ref：当 `autoScroll === true` 且位于第 1 页时，新日志抵达后平滑滚动表格容器到顶部（`scrollTop = 0`）。

### 5.2 HeroUI v3 与样式对齐
- 复用并按需扩展 `PageBanner`（保持与媒体库/STRM 页一致的视觉基线，支持传入可选插画或默认统一风格）。
- 严格使用 HeroUI v3 组件（`Card`, `Select`, `SearchField`, `Switch`, `Checkbox`, `Button`, `Dropdown`, `Modal`, `RadioGroup`, `Table`, `Chip`, `Link`）与语义 CSS 变量（`bg-surface`, `bg-danger-soft`, `text-foreground`, `text-muted` 等），保证浅色与深色主题下文字对比度与错误行高亮均清晰可读。
- 下载操作使用 `fetch`（或携带 `same-origin` Cookie 的 `fetch` 校验状态与 `X-Export-Truncated` 头后触发 `Blob` 下载），从而在会话失效（401）或服务异常（503）时能优雅展示中文错误提示而不是裸跳浏览器错误页！

## 6. Trade-offs & Rollback Considerations

- **为何批刷后再推 SSE（200ms 延迟 vs 立即内存推送）**：200ms 批刷保证单连接 SQLite 吞吐稳定、不阻塞业务线程，且 SSE 事件携带真实数据库 `id`，前端按 `id` 精确去重，避免刷新闪动或重复行。
- **回滚策略**：
  - 数据库迁移 `00009_runtime_logs.sql` 仅新增独立表，不修改任何既有表结构，具备安全 `Down` 语句；
  - 现有 `stdout` + `lumberjack` 日志链路完全保留；
  - 若上线后出现异常，可直接回退代码与 OpenAPI 生成文件，不影响已有认证、115、媒体库、STRM 或任务中心数据。
