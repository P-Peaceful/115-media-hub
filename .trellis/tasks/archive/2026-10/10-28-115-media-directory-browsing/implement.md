# 实施计划：115 媒体目录浏览

## 顺序与检查点

1. **契约**：在 `api/openapi.yaml` 定义经管理员认证的单页目录读取接口、参数边界、错误响应、文件条目/面包屑 DTO；运行 `make generate`，审阅 `backend/internal/http/generated/api.gen.go` 与 `frontend/src/shared/api/generated/schema.d.ts`。
2. **115 adapter**：在 `backend/internal/adapter/drive115` 增加单页 list/search/stat 能力，显式注入 context、timeout 和凭证；映射稳定 ID、大小、文件类型和时间；利用 list response 的 Count，不全目录扫描。由于 SDK `GetFiles` 未暴露 search/cur 字段，用 credential client 的 SDK request 层访问 `/files`，显式设置 `cur=1`、`cid`、偏移、限量、搜索词/类型；静态审阅最终请求参数组装，确认范围和分页契约。
3. **业务 service/HTTP**：新增 browse service 负责校验、连接读取、adapter 调用、错误分类与安全结构化日志；handler 仅绑定参数、调用 service、映射 DTO/HTTP 状态；router/app 装配并确认管理员认证覆盖。
4. **前端数据边界**：新增 `features/browse` API/query/mapping；URL 管目录 ID、筛选、查询、offset/page；query key 含所有结果参数；处理无连接、加载、空页、旧数据刷新、当前页错误/重试。
5. **前端视图**：把 `/browse` 接入路由与工作台；按图 05 调整 browse 内容区宽度、筛选条、面包屑、选择行、表格、分页、返回上级；保留两个未接入按钮为禁用状态；不实现图 06 或任何多页预取。
6. **对照与收尾**：以图 05 同尺寸 1440×1024 打开本地页面逐区比对，检查窄屏和键盘焦点；运行可执行的格式、静态分析、类型和构建检查；不运行测试套件（本阶段未获用户测试/验证请求）。审阅 diff，检查敏感数据没有进入 API schema、浏览器持久化或日志。

## 范围边界

- 预期代码范围：`api/openapi.yaml`、生成的 Go/TS API 文件、`backend/internal/adapter/drive115`、新增 `backend/internal/service/browse`、HTTP handler/router/app composition、`frontend/src/features/browse`、前端 router/layout/CSS，以及与接口同步的项目说明（如需）。
- 不修改连接授权流程、DB schema、媒体库/生成/任务业务，不修改设计图、部署模式或添加第三方依赖。
- 115 SDK `/files` 搜索使用 `cur=1` 保证当前目录直接子项范围；Stat 父链需在 adapter 层检查和规范化根/当前目录边界。

## 验证命令（实施阶段）

- `make generate`（契约变化）
- `cd backend && gofmt -w <改动的Go源文件>`
- `cd backend && go vet ./...`
- `pnpm --dir frontend typecheck`
- `pnpm --dir frontend lint`
- `pnpm --dir frontend peers check`
- `pnpm --dir frontend build`
- `make build` 或 `make smoke` 仅当嵌入构建/静态资源/服务端路由改动需要集成确认时（这两个目标会编译/启动应用但不会跑 Go/前端测试套件；执行前确认实现的目录访问不会触碰真实 115 账户）。

## 回滚点

- 在未合并的单个任务中，通过回退 browse feature/API/routes/adapter/service 移除新能力；不需 DB down migration。
- API 变更与生成文件始终同提交；若 SDK 搜索语义无法满足当前目录约束，停止该检查点，不在 UI 层临时扩大搜索范围。
