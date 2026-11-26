# 日志模块实施计划（Implement）

## Ordered Implementation Checklist

### Phase A：契约与数据库基础设施
1. **更新 OpenAPI 契约** (`api/openapi.yaml`)
   - 新增 `/api/v1/logs`、`GET /api/v1/logs/stream`、`GET /api/v1/logs/export`、`POST /api/v1/logs/clear`。
   - 新增 `RuntimeLogLevel`、`RuntimeLogModule`、`RuntimeLogEntry`、`RuntimeLogPage`、`RuntimeLogRange`、`RuntimeLogExportFormat`、`ClearRuntimeLogsScope`、`ClearRuntimeLogsRequest`、`ClearRuntimeLogsResponse` 等 Schema。
   - 运行 `make generate && make check-generated` 生成 Go 与 TypeScript 类型。
2. **新增 SQLite 迁移** (`backend/migrations/00009_runtime_logs.sql`)
   - 创建 `runtime_logs` 表及时间/级别/模块/任务组合索引。

### Phase B：后端领域、仓储与服务层
3. **实现领域包** (`backend/internal/domain/logs/`)
   - 定义 `Entry`、`Filter`、`Page`、`ClearScope`、`ExportFormat`、合法模块/级别集合。
   - 实现脱敏净化器（Cookie/Token/Authorization/临时链接脱敏）、白名单属性提取及中文可读消息格式化纯函数 + 完整单元测试。
4. **实现 SQLite 仓储** (`backend/internal/repository/sqlite/runtime_logs.go`)
   - `InsertRuntimeLogs(ctx, entries)`：单事务批量插入并返回分配的自增 `id`。
   - `ListRuntimeLogs(ctx, filter)`：时间范围、级别、模块、任务、转义 LIKE 关键词与不透明游标倒序分页查询。
   - `ClearRuntimeLogs(ctx, before, scope, audit)`：单事务删除匹配运行日志并强制写入 `audit_records`，审计失败整笔回滚。
   - `PruneRuntimeLogs(ctx, cutoff, maxRows, batchLimit)`：有界分批裁剪。
   - 编写 `runtime_logs_test.go`（覆盖批量写、组合筛选、游标分页、事务回滚、裁剪及不影响 `audit_records`/`task_logs`）。
5. **实现日志服务与 `slog` 采集器** (`backend/internal/service/logs/`)
   - 实现有界入库队列、后台批刷与定时裁剪协程、`Close(ctx)` 优雅排空。
   - 实现 `CollectorHandler`（透传原 handler + 噪声过滤 + 防递归 + 满队列丢弃限频诊断）。
   - 实现有界 SSE 订阅广播中心（最大订阅数、非阻塞慢客户端保护、清理事件通知）。
   - 实现 `List`、`Export`（流式 10,000 条上限与终端日志）、`Clear`（参数校验、事务清理与终端日志）。
   - 编写 `service_test.go`（含并发 `-race`、满队列不阻塞、防递归、脱敏、中文格式化、关闭刷盘验证）。

### Phase C：后端 HTTP 传输层与进程装配
6. **实现 HTTP 与 SSE 处理器** (`backend/internal/http/logs.go`, `backend/internal/http/logs_stream.go`)
   - 注册 `/api/v1/logs*` 路由到 `router.go` 与 `AuthOptions`。
   - 校验查询/请求体参数、游标、范围与导出格式；SSE 严格执行会话复核、15s 心跳与 5s 单帧写超时。
   - 编写 `logs_test.go`（覆盖未认证 401、缺 CSRF 403、多维筛选、SSE 推送与注销断连、`.log`/`.jsonl` 导出与截断头、清理审计回滚 503）。
7. **接入应用启动与关闭生命周期** (`backend/internal/app/app.go` 及任务日志桥接)
   - 在 `app.New` 中于 `sqlite.Open` 后初始化 `logs.Service` 并用 `WrapLogger` 包装 `slog.Logger` 注入全部业务模块与路由。
   - 将任务中心 `InsertJobLog` / 任务关键日志桥接到运行日志采集，保证任务 ID 关联与不重复刷屏。
   - 在 `App.Serve` 优雅关闭阶段、停止 HTTP 与任务后调用 `a.logs.Close(shutdownCtx)` 再释放数据库。
   - 补充 `backend/internal/app/app_test.go` 集成验证。

### Phase D：前端页面、导航与测试
8. **前端 API、实时流与查询层** (`frontend/src/features/runtime-logs/`)
   - `api.ts`：强类型运行日志列表查询、清理 mutation、流式 Blob 下载（解析 `X-Export-Truncated` 与 401/错误码）。
   - `log-stream.ts`：运行时校验 SSE `log.created` / `log.cleared` 负载的 `EventSource` hook，支持筛选切换重连、卸载释放。
   - `log-labels.ts`：模块中文标签、时间范围选项、级别样式映射与任务链接格式化。
9. **前端界面与路由接入**
   - `runtime-logs-page.tsx`、`runtime-logs-table.tsx`、`clear-logs-dialog.tsx`：严格对照设计图实现 HeroUI v3 页面、5 维筛选栏、2 行操作栏（自动滚动、高亮错误、刷新、下载日志下拉、清理日志弹窗）、倒序五列表格与分页。
   - 更新 `frontend/src/app/app.tsx` 注册 `/logs`，更新 `frontend/src/app/app-shell.tsx` 开放「运行日志」侧栏入口与面包屑归属。
   - 补充 Vitest 单元测试与 Playwright E2E 测试（含筛选、错误高亮、自动滚动实时日志、导出、清理弹窗与浅/深色主题）。

### Phase E：质量检查与文档同步
10. **规范检查与文档更新**
    - 检查所有新增/修改手写 Go 文件的中文职责与导出/主要方法注释。
    - 运行全量质量门禁与生成一致性校验。
    - 按需更新 `.trellis/spec/backend/logging-guidelines.md` 与前端相关规范引用真实源码路径。

## Validation Commands

```bash
# 1. OpenAPI 生成与一致性校验
make generate && make check-generated

# 2. 后端全量质量门禁（fmt, vet, test -race, golangci-lint）
cd backend && make check

# 3. 前端质量门禁（format, lint, typecheck, vitest, build）
cd frontend && pnpm format:check && pnpm lint && pnpm typecheck && pnpm test:run && pnpm build

# 4. 跨项目总检查
make check

# 5. 运行日志 E2E 浏览器验证（及侧栏回归）
cd frontend && pnpm test:e2e -- e2e/runtime-logs.spec.ts
```

## Risky Files & Rollback Points

| 风险文件 | 风险点 | 缓解与回滚点 |
| --- | --- | --- |
| `backend/internal/app/app.go` | 启动包装 logger 与关闭时 `logs.Close()` 顺序若晚于 `database.Close()` 会导致刷盘失败 | 在 `App.Serve` 停止 HTTP/jobs 后、`Close()` 前显式刷盘，并用 `app_test.go` 验证重启持久化 |
| `backend/internal/service/logs/` | 单连接 SQLite 下同步写库易死锁，或内部日志触发递归 | 严格异步有界队列 + `module=="logging"` 内部操作绕过采集 + `-race` 并发单测 |
| `backend/internal/http/router.go` | 中间件对 `/api/v1/logs*` 写访问日志导致日志页自我刷屏 | 噪声过滤显式跳过常规 HTTP 2xx/3xx 及 `/health/*` |
| `frontend/src/app/app-shell.tsx` | 修改侧栏导航可能影响既有侧栏 E2E 断言 | 完成后同步运行既有侧栏/导航 Playwright 用例验证 |
| `api/openapi.yaml` | 契约改动后忘记同步生成文件 | 每次改动契约立即运行 `make generate && make check-generated` |

## Follow-up Checks Before `task.py start`

- [x] `prd.md` 已通过无损收敛（无未解决阻塞问题、无重复段落）。
- [x] `design.md` 与 `implement.md` 已完整落盘。
- [ ] 向用户展示最终规划总结，等待用户在下一条消息中明确批准后再执行 `task.py start` 与编码实现。
