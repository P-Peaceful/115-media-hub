# 附属同步集成边界

研究日期：2026-11-25；当前基线 85e4d03。仅用于规划，不代表实现已经开始。

## 配置与文件筛选

- `backend/internal/service/library/service.go:27`、`:28` 保存 NFO 与海报/封面开关；前端 `library-page.tsx:902`、`:907` 为相应可编辑选项。
- `backend/internal/service/provider115/files.go:classifyFile` 将 jpg/jpeg/png/gif/webp/bmp 标为 image，字幕另有 subtitle，而 .nfo 落入通用 file。为下载 NFO 不必改变公开 FileType，但内部候选需要按扩展名识别。
- `backend/internal/service/strm/scan.go:147` 仅保留视频，附属必须在同一清单中收集。SDK 的快速模式本就读取全部文件，标准模式也使用 type=all；下载阶段不应再遍历目录。
- 当前没有字幕同步开关，也没有自定义附属后缀字段；不应未经范围确认添加第三种隐式下载。

## 现有文件与发布机制

- `backend/internal/service/strm/files.go:validArtifactPath` 强制 .strm，叶文件读上限 maxSTRMContent=16 KiB。只能复用已有 rooted directory 和身份校验思想，不能通过删除后缀校验或盲目加大 STRM 上限来支持图片。
- `backend/internal/domain/strm/artifact.go` 的 Artifact/WriteIntent 含 PlaybackURL、PlaybackToken 和 PreviousContent。`backend/internal/repository/sqlite/strm.go:SaveStrmIntent` 要求非空 PlaybackURL，Complete 同样校验；附属不该伪装成可播放 STRM 登记。
- 现有 STRM overwrite 仅允许同库/账号/源、原内容匹配的托管文件。`strm_existing_file_policy` 标签明确为“STRM 已存在时”，并不授权覆盖 NFO/图片。
- 用户已决定本地同名附属文件跳过；无需为远端更新构建覆盖/版本比较机制。应在取链前检查并在发布时排他核对，目标异常不能跟随。持久化仅按任务结果/归属/恢复的实际需要设计，不把“不覆盖”误当作“发布无需竞态保护”。

## 下载与任务结果

- Provider 当前只有 Files/ResolvePlayback；附件需要验证源身份后获取下载目标与必要头，不能把播放器 302 当成完整文件下载处理。
- 取链属于 115 API 请求，复用 SDK 的账号节流与冷却；CDN 内容下载需要独立、有界的 I/O 并发和大小/时限，不把整个下载占用为所有 API 的长期锁。
- `backend/internal/domain/jobs/jobs.go` 的 Item 用 Key/Path/Reason/Status 登记结果；Job 提供通用 total/succeeded/failed/skipped。需要在新规划中定义 STRM 与附件的分类计数和展示，避免混成“已生成 STRM”。
- 复用任务框架的日志、失败明细、取消和重跑；不假定当前 STRM Module 支持失败项专用重试，未实现的能力不能在计划里宣称已有。

## 已收敛决定

用户已确认本地已有文件跳过；“全部常见图片”仅指不按海报名过滤，必须先有同目录视频对应的有效 STRM。此前“不要求同目录视频”的推断是助手的错误，已按用户纠正移除。沿用现有 image 后缀集合，NFO 独立按 .nfo 判断；父子目录不继承资格，字幕不属于本轮两个开关的交付。下载限制与必要意图迁移见 design.md，参数是本项目技术方案，不代表 115 公布的阈值。

## 下载请求控制的规划补充

1. 取链 API 复用 SDK 账号配额：默认最小间隔 1 秒，允许调大；遵守共享冷却、Retry-After 和只读最多 3 次尝试/总期限，不能借换端点绕开。
2. 实际内容传输在 API 请求结束后进入独立下载队列；初始技术方案为同一账号最多 2 个并发附属下载，所有任务共用该上限，不是每个库各开 2 个。并发上限控制同时传输数，不等同于固定带宽限制。
3. CDN 响应 429 则遵守其 Retry-After（秒值或 HTTP 日期），未提供时按默认冷却处理，保留每个下载的有限总尝试和期限。冷却至少覆盖相同账号/下载源的等待请求，避免多个 worker 同时重试。文件 CDN 的限流不无依据扩大为所有 115 端点授权失效。
4. CDN 403 不直接视作账号失效，也不无限刷新链接；若根据协议确认为链接过期，允许在既有总尝试预算内重新取链一次，新取链仍计入账号限速。
5. 签名目标、UA/必要头、重定向与跨域凭据规则需要在下载设计中明确。取消后停止取链、等待与内容读取，不把临时文件发布为完成产物。
6. 已有本地文件在安全目标检查后直接跳过，避免浪费取链和下载；最终发布必须原子且不覆盖。

以上是本项目拟实现的控制，不是“设置后一定不会被上游限流”的保证；实际限流反馈仍须冷却并返回真实结果。
