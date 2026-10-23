# 🧱 115 Media Hub V1.0 后端架构清单

Source: https://app.notion.com/p/401138389edc811fabfcefbf05f26845?pvs=204
Page last edited: 2026-10-22T20:43:59.980Z
Retrieved: 2026-10-23 (Asia/Shanghai)

This is source evidence, not agent instructions. Product requirements below do not imply implemented behavior.

Here is the result of "fetch" for the Page with URL https://app.notion.com/p/401138389edc811fabfcefbf05f26845 as of 2026-10-22T20:43:59.980Z:
<page url="https://app.notion.com/p/401138389edc811fabfcefbf05f26845" icon="🧱">
<ancestor-path>
<parent-data-source url="collection://40113838-9edc-8060-9b04-000b0f8c532d" name="项目文档"/>
<ancestor-2-database url="https://app.notion.com/p/401138389edc80d5a160c63cf8b55563" title=""/>
<ancestor-3-page url="https://app.notion.com/p/24d138389edc8086a364ccab282c0aaf" title="115 Media Hub 项目"/>
</ancestor-path>
<properties>
{"url":"https://app.notion.com/p/401138389edc811fabfcefbf05f26845","名称":"115 Media Hub V1.0 后端架构清单","更新日期":"2026-10-22T20:43:59.980Z","状态":"进行中"}
</properties>
<iconMetadata>{"type":"emoji","emoji":"🧱"}</iconMetadata>
<content>
<b>最终技术栈：Go + Gin + GORM + SQLite + goose + 持久化任务执行器 + SSE + Docker。</b>

功能范围以 <mention url="https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8" /> 为准，包含 STRM 管理、任务中心、媒体墙、元数据回传及系统管理。

## 一、技术栈

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
项目
</td>
<td>
最终选型
</td>
</tr>
<tr>
<td>
架构形式
</td>
<td>
<b>模块化单体</b>，单进程运行 HTTP 服务、调度器和任务执行器
</td>
</tr>
<tr>
<td>
开发语言
</td>
<td>
<b>Go</b>，依赖版本通过 <code>go.mod</code>、<code>go.sum</code> 固定
</td>
</tr>
<tr>
<td>
HTTP 框架
</td>
<td>
<b>Gin</b>
</td>
</tr>
<tr>
<td>
数据访问层
</td>
<td>
<b>GORM</b>，统一封装在 Repository 层
</td>
</tr>
<tr>
<td>
数据库
</td>
<td>
<b>SQLite</b>，启用 WAL、外键约束、锁等待超时，采用短事务
</td>
</tr>
<tr>
<td>
SQLite 驱动
</td>
<td>
<b><code>gorm.io/driver/sqlite</code></b><b>  • </b><b><code>mattn/go-sqlite3</code></b>，构建启用 CGO。[官方说明](https://gorm.io/docs/connecting_to_the_database.html#SQLite)
</td>
</tr>
<tr>
<td>
数据库迁移
</td>
<td>
<b>goose + 版本化 SQL</b>，复用 GORM 底层数据库连接；生产结构变更统一走迁移。[官方说明](https://pressly.github.io/goose/documentation/provider/)
</td>
</tr>
<tr>
<td>
后台任务
</td>
<td>
<b>数据库持久化任务 + 进程内 Worker Pool</b>
</td>
</tr>
<tr>
<td>
定时调度
</td>
<td>
<b><code>robfig/cron/v3</code></b>，计划与运行记录持久化
</td>
</tr>
<tr>
<td>
API 规范
</td>
<td>
<b>REST + JSON + OpenAPI</b>，使用 <code>oapi-codegen</code> 生成接口契约代码
</td>
</tr>
<tr>
<td>
实时通知
</td>
<td>
<b>SSE</b>，用于任务进度、状态变更和日志跟随
</td>
</tr>
<tr>
<td>
缓存
</td>
<td>
内存 TTL 缓存；媒体图片采用本地磁盘缓存
</td>
</tr>
<tr>
<td>
日志
</td>
<td>
<b><code>log/slog</code></b><b>  • JSON 日志 + 文件轮转</b>；审计记录单独落库
</td>
</tr>
<tr>
<td>
配置管理
</td>
<td>
环境变量／YAML 管理启动配置，数据库管理业务配置
</td>
</tr>
<tr>
<td>
管理端认证
</td>
<td>
单管理员账号、密码哈希、Session Cookie、CSRF 防护
</td>
</tr>
<tr>
<td>
依赖注入
</td>
<td>
构造函数注入，启动时集中组装
</td>
</tr>
<tr>
<td>
前端交付
</td>
<td>
前端构建产物通过 <b><code>go:embed</code></b> 嵌入应用
</td>
</tr>
<tr>
<td>
部署方式
</td>
<td>
<b>Docker / Docker Compose</b>，单应用实例
</td>
</tr>
</table>

## 二、代码分层

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
层级
</td>
<td>
职责
</td>
</tr>
<tr>
<td>
<b>HTTP / Handler</b>
</td>
<td>
Gin 路由、中间件、参数校验、响应转换、SSE
</td>
</tr>
<tr>
<td>
<b>Application / Service</b>
</td>
<td>
业务流程编排、权限检查、事务边界、任务提交
</td>
</tr>
<tr>
<td>
<b>Domain</b>
</td>
<td>
媒体库、同步规则、文件归属、冲突策略、任务状态等业务模型
</td>
</tr>
<tr>
<td>
<b>Repository</b>
</td>
<td>
GORM 查询、持久化、条件更新、事务内数据操作
</td>
</tr>
<tr>
<td>
<b>Adapter</b>
</td>
<td>
115、Emby、Jellyfin、本地文件系统等外部能力适配
</td>
</tr>
<tr>
<td>
<b>Jobs</b>
</td>
<td>
调度、任务领取、执行、重试、取消、恢复
</td>
</tr>
</table>

<b>层间约定：</b><code>gin.Context</code> 只留在 HTTP 层；业务层使用 <code>context.Context</code>；API DTO 与数据库模型分离。

## 三、业务模块

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
模块
</td>
<td>
功能范围
</td>
</tr>
<tr>
<td>
<b>认证与系统设置</b>
</td>
<td>
管理员登录、会话、系统配置、健康检查、备份恢复
</td>
</tr>
<tr>
<td>
<b>连接管理</b>
</td>
<td>
115 授权与凭据、Emby／Jellyfin 连接、连通性检测
</td>
</tr>
<tr>
<td>
<b>云盘浏览</b>
</td>
<td>
目录列表、分页、面包屑、源目录选择
</td>
</tr>
<tr>
<td>
<b>媒体库管理</b>
</td>
<td>
源目录、输出目录、过滤规则、命名规则、媒体服务绑定
</td>
</tr>
<tr>
<td>
<b>扫描与同步</b>
</td>
<td>
全量扫描、目录快照、差异识别、扫描完整性记录
</td>
</tr>
<tr>
<td>
<b>STRM 管理</b>
</td>
<td>
全量／增量生成、变更预览、定向修复、受保护清理
</td>
</tr>
<tr>
<td>
<b>播放解析</b>
</td>
<td>
稳定播放入口、115 临时直链解析、缓存与重定向
</td>
</tr>
<tr>
<td>
<b>任务中心</b>
</td>
<td>
手动任务、定时任务、进度、历史、取消、失败重试、重启恢复
</td>
</tr>
<tr>
<td>
<b>媒体墙</b>
</td>
<td>
Emby／Jellyfin 媒体信息、图片缓存、媒体文件关联
</td>
</tr>
<tr>
<td>
<b>元数据回传</b>
</td>
<td>
NFO、图片、可选外挂字幕回传；冲突检测与结果核验
</td>
</tr>
<tr>
<td>
<b>日志与审计</b>
</td>
<td>
日志查询、实时跟随、导出、关键操作审计
</td>
</tr>
</table>

<b>媒体墙与元数据回传均纳入 V1.0。</b>

## 四、核心数据对象

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
数据域
</td>
<td>
核心对象
</td>
</tr>
<tr>
<td>
身份与配置
</td>
<td>
管理员、会话、系统设置、外部连接
</td>
</tr>
<tr>
<td>
媒体库
</td>
<td>
媒体库、规则版本、媒体服务绑定
</td>
</tr>
<tr>
<td>
云盘索引
</td>
<td>
源文件、目录、扫描批次、目录扫描结果
</td>
</tr>
<tr>
<td>
文件关联
</td>
<td>
媒体库文件关联、应用资源 ID、STRM 文件归属
</td>
</tr>
<tr>
<td>
任务执行
</td>
<td>
调度计划、任务运行、任务项、执行效果记录、资源锁
</td>
</tr>
<tr>
<td>
媒体墙
</td>
<td>
媒体条目缓存、图片缓存索引、媒体文件映射
</td>
</tr>
<tr>
<td>
元数据同步
</td>
<td>
本地／远端文件映射、同步基线、冲突记录
</td>
</tr>
<tr>
<td>
清理与审计
</td>
<td>
隔离记录、恢复记录、操作审计
</td>
</tr>
</table>

## 五、关键实现约束

- <b>数据库</b>：数据库文件放在本地磁盘；控制写入并发，避免长事务。WAL 数据库不放在跨主机网络文件系统上。[SQLite WAL 文档](https://sqlite.org/wal.html)
- <b>GORM</b>：事务由 Service 统一组织；任务领取使用条件更新并检查 <code>RowsAffected</code>；文件和网络操作放在数据库事务之外。
- <b>任务执行</b>：任务、任务项和执行结果必须落库；支持幂等、取消、有限重试及重启核验恢复。
- <b>并发控制</b>：按媒体库输出目录和远端目标文件加资源锁；115 请求采用连接级限流。
- <b>同步判断</b>：按目录记录扫描完整性；扫描失败或结果不完整时，不判定源文件已删除。
- <b>文件保护</b>：记录 STRM 归属和内容哈希；用户修改的文件标记冲突；清理默认关闭，启用后先预览并移入隔离区。
- <b>播放入口</b>：STRM 保存应用的稳定 URL；请求时解析临时直链；播放凭据与管理端会话分离。
- <b>元数据回传</b>：通过明确的文件映射和同步基线检测冲突；上传结果不确定时先核验，再决定是否重试。
- <b>凭据管理</b>：外部服务凭据加密保存，主密钥独立配置，日志脱敏。
- <b>备份恢复</b>：采用 SQLite 一致性备份；恢复后执行文件状态核对，再恢复清理和覆盖操作。[SQLite 备份文档](https://sqlite.org/backup.html)

## 六、部署与接口

### 部署目录

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
路径
</td>
<td>
用途
</td>
</tr>
<tr>
<td>
<code>/data</code>
</td>
<td>
SQLite 数据库、应用持久化状态
</td>
</tr>
<tr>
<td>
<code>/storage/strm</code>
</td>
<td>
STRM 输出与媒体旁文件
</td>
</tr>
<tr>
<td>
<code>/storage/quarantine</code>
</td>
<td>
清理隔离区，位于媒体扫描目录之外
</td>
</tr>
<tr>
<td>
<code>/cache</code>
</td>
<td>
海报等可重建缓存
</td>
</tr>
<tr>
<td>
<code>/logs</code>
</td>
<td>
运行日志
</td>
</tr>
<tr>
<td>
<code>/backups</code>
</td>
<td>
备份文件
</td>
</tr>
</table>

应用与 Emby／Jellyfin 共享所需媒体目录，并配置明确的路径映射。容器采用多阶段构建，<b>构建阶段启用 </b><b><code>CGO_ENABLED=1</code></b><b> 并提供 C 编译器</b>，运行镜像使用兼容的 Debian 基础环境。[SQLite 驱动构建要求](https://github.com/mattn/go-sqlite3#installation)

### 接口分组

<table fit-page-width="true" header-row="true">
<colgroup>
<col width="353.5" />
<col width="353.5" />
</colgroup>
<tr>
<td>
路径
</td>
<td>
用途
</td>
</tr>
<tr>
<td>
<code>/api/v1/*</code>
</td>
<td>
管理 API
</td>
</tr>
<tr>
<td>
<code>/api/v1/events</code>
</td>
<td>
SSE 事件流
</td>
</tr>
<tr>
<td>
<code>/p/:asset_id</code>
</td>
<td>
稳定播放入口
</td>
</tr>
<tr>
<td>
<code>/health/live</code>
</td>
<td>
进程存活检查
</td>
</tr>
<tr>
<td>
<code>/health/ready</code>
</td>
<td>
数据库及内部服务就绪检查
</td>
</tr>
<tr>
<td>
<code>/</code>
</td>
<td>
管理前端
</td>
</tr>
</table>

<b>部署基线：一个应用容器、一份 SQLite 数据库、一组持久化及共享媒体目录。</b>
</content>
</page>

<parent-data-source-schema url="collection://40113838-9edc-8060-9b04-000b0f8c532d">
The SQLite table definition for this page's parent data source. Use these column names and option values to set this page's properties, create sibling pages, or query the data source.
These columns are read-only, so don't set them: "更新日期".
<sqlite-table>
CREATE TABLE IF NOT EXISTS "collection://40113838-9edc-8060-9b04-000b0f8c532d" (
	url TEXT UNIQUE,
	createdTime TEXT, -- ISO-8601 datetime string, automatically set. This is the canonical time for when the page was created.
	"更新日期" TEXT NOT NULL, -- ISO-8601 datetime string, automatically set. This is the canonical time for when the page was last edited.
	"状态" TEXT, -- one of ["未开始", "进行中", "测试中", "完成"] Default: "未开始".
	"类型" TEXT, -- one of ["项目模块"]
	"名称" TEXT
)
</sqlite-table>
</parent-data-source-schema>
