# 技术设计：115 媒体目录浏览

## 目标与边界

交付一条从工作台到 115 当前目录的真实读取链路：管理员浏览目录，按需请求一个服务端分页，能够进入子目录、搜索和筛选，并在当前页失败时保留可用内容、重试当前页。UI 以图 05 为视觉基线。图 06 与其部分加载/前三页并发逻辑不实现。

媒体库创建和生成范围按钮不触发后端业务，不显示成功结果；保留图 05 中位置和文字，通过禁用状态及“暂未接入”辅助说明体现范围。

## 已选方案及权衡

1. **按需单页读取（采用）**：每个 URL 对应一页；目录浏览 API 单次向 115 拉取一个分页。请求少、数据有界、失败边界清晰，也符合用户明确选择。未访问的其他页不会被预先请求或提示失败。
2. **初始并行读取前三页（放弃）**：更容易显示图 06 的第三页失败，但进入目录时多发请求且要管理多页局部状态；用户已明确抛弃该设计。
3. **全目录读取后浏览器分页（放弃）**：实现简单但无界读放大，违反服务端分页要求；SDK 的 `ListWithLimit` 会遍历所有目录页，也不适用于该页面。

## 前后端边界

### API 契约

在 `api/openapi.yaml` 增加管理员保护的只读接口：

`GET /api/admin/drive/115/directories/{directoryId}`

读取目录名称和祖先面包屑；根目录 `0` 使用应用固定名称“我的网盘”，不请求上游 Stat。

`GET /api/admin/drive/115/directories/{directoryId}/entries`

entries 参数：`directoryId`（`0` 表示根目录）、`offset`、受限的 `limit`（设计图每页 6 条）、`type`（稳定枚举）及可选 `q`。响应包括：当前目录 ID、当前页条目、当前查询结果总数、offset/limit。条目包括稳定 ID、父目录 ID、名字、是否目录、大小（字节）、修改时间和展示文件类型。不返回 pick code、SHA1、cookie、115 凭证或上游原始 JSON。

不额外创建“创建媒体库/加入生成范围” API。现有管理员路由组统一认证保护；GET 不需 CSRF token。输入校验失败返回 400；无连接返回 404；上游目录读取/账号连接错误以不泄密的统一错误结构返回 503；缺失或不可读目录按已确认的 SDK 错误分类返回明确 404/503。

### 数据流

1. React Router 从 URL 解析目录 ID、分页、过滤和搜索条件；本地状态只保存当前页选择等临时状态。
2. TanStack Query key 包含上述所有查询维度。切换查询条件立即清除选择并回到第一页；scope key 隔离旧请求，保留同一 key 的旧数据用于后台刷新失败时继续显示。
3. 前端目录元数据 query 以 directory ID 为 key。站内进入子目录时，使用已加载父路径和父目录列表返回的子目录 ID/名称写入 Query 缓存，避免每层导航都依赖不稳定的 Stat；直接打开/刷新链接时仍调用目录信息接口。直达目录的 Stat 失败不阻断条目浏览，进入子目录时对未知的当前路径段使用省略号标识。页码改变时不重复请求目录 Stat。
4. HTTP handler 绑定/验证参数，调用 browse service；不在 Gin 层处理 115 SDK 对象。
5. browse service 从 SQLite 读取当前有效连接凭证（不新增表/迁移），调用 drive115 只读目录 adapter，映射为安全的业务响应并分类错误。
6. adapter 由当前凭证创建临时 SDK client，使用 `client.NewRequest()`、`driver.ApiFileList`、`driver.FileListResp`、`driver.CheckErr` 发起有界 GET 请求，并用 `driver.File.From` 映射本页结果，保留 `Count`；不要调用会丢失总数的 `ListPage`，也不要调用会遍历全目录的 `ListWithLimit`。目录信息接口调用一次 SDK `Stat`，把父级链映射为面包屑；根目录使用固定 crumb。

### 搜索和文件类型

通过 SDK 连接上的 `/files` 列表接口承载目录分页、搜索和类型筛选：传 `cid`、`offset`、`limit`、可选 `search_value`/`type`，并在查询时显式传 `cur=1`，限定当前目录而非递归其子目录。实现使用 SDK `Pan115Client.NewRequest()`、`driver.ApiFileList`、`driver.FileListResp` 和 `driver.CheckErr`，因为 SDK v1.3.5 的 `GetFiles` 没有暴露 `search_value`/`cur` 入参。类型枚举按该 `/files` 端点定义显式映射；未知/空值拒绝。公开资料把 `/files` 的 `cur` 定义为“是否只搜索当前目录”，并指出某些筛选在 `cur=0` 时会遍历目录树，因此必须固定 `cur=1`；实现阶段仍须通过可替换 HTTP transport 的 adapter 请求断言核对查询参数。

### 面包屑和计数

目录列表响应的 `count` 用作当前查询总数；该响应默认包含文件夹且总数对应分页结果，实际请求显式设置 `show_dir=1`。SDK `Stat` 可以返回直接文件数、子文件夹数及父级路径；用于面包屑/文件夹摘要时只需在目录改变时取一次。不要基于 SDK `ListPage` 的切片长度推导全量计数。

## 日志、隐私、错误

- 继续复用 router 已有的 method、route template、status、duration HTTP 请求日志。
- browse service 在关键操作边界记录结构化开始/结果/失败日志，字段限于 offset、limit、类型枚举、是否有关键词、返回数量、耗时、安全错误分类；不记录关键词、目录/文件名、原始 URL、Cookie、凭证、PickCode、SHA1 或完整上游 body。
- 一次失败只在归属层记录一次，HTTP 错误映射不得重复写相同堆栈；错误面向客户端使用固定中文文案/稳定分类，后台日志附 `errors.Is` 得到的安全原因类别。
- 对 115 Count 与本页条目不一致、越界 offset 等边界保留真实未知/空含义，不捏造页数或进度。

## 前端视觉与行为

- 新建 `features/browse` 的 API 边界、列表模型、页面和样式；共用 `request` API client、Query provider、HeroUI v3/Lucide，不把数据放入 Zustand。
- 将图 05 的信息层次带入工作台：顶部标题/描述和主操作、连接账号选择、当前目录搜索、类型选择、面包屑、选择批次操作、列头/条目、底部分页和返回上级。与当前 `AppLayout` 的固定宽度 `max-w-5xl` 主区冲突，需为 browse 路由开放足够内容宽度，并保持其他页面现有布局。
- 顶栏任务运行数目前没有真实任务 API，沿用现有“—”未知计数，不照抄图片中的静态 `2` 以免伪造运行状态。
- 列表严格只呈现一个目录；点击文件夹导航到稳定 ID；面包屑优先复用服务端成功返回的父路径和列表中的子目录名称，直达/刷新时再读取服务端父链。
- `q`、`type`、`offset/page`、`directoryId` 由 URL 表达；页选择保留在组件 state，跳页/换目录/换过滤即清空选择。
- 状态包含首次加载骨架/占位、空目录、已加载、保留旧数据刷新、单页错误 + 重试、未配置 115 连接。无多页预取、部分加载横幅或多页失败状态。
- 设计图中两项未实现操作保持在图示位置但禁用，旁有明确“暂未接入”提示；不得按钮点击后伪造添加成功。
- 采用可访问语义和键盘焦点，响应窄屏并处理长中文文件名；图标按钮提供名称。

## 持久化、兼容与回滚

- 不需数据库 schema 变更；只复用已有 active 115 connection 与凭证。
- 新增字段/路径全部来自 OpenAPI，运行 `make generate` 更新 Go handler interface 与 TS schema，不手改生成文件。
- 回滚可移除新增路由/feature 和 API 操作；无需迁移降级。不得更改既有 115 认证/API 行为。

## 风险与待执行技术核验

- SDK `GetFiles` 未暴露搜索词或 `cur` 参数，故 adapter 需要调用 SDK client 的底层请求接口组装单页 `/files` 请求；实施时静态审阅 offset/limit/search/type/cur 参数，不得误用 `ApiFileSearch` 造成递归范围不清或分页总数来源不一致。
- `Stat` 的 `Parents` 是否包含根目录/当前目录，需验证以确保根路径和面包屑无重复/缺项。
- 当前测试配置不含 DOM/browser suite；视觉需手动在 1440×1024 对照图 05 并检查窄屏。检查命令不运行测试套件，除非用户后续明确要求。
