# 附属同步复核记录

## 复核范围

复核当前附属同步实现及其交接边界，重点检查：同轮扫描复用、快慢扫描投影、同目录有效 STRM 准入与发布前复核、已有文件零网络请求、下载取链/内容限流、凭据变化、根目录与临时文件安全、意图迁移恢复、任务分类统计和中文注释。

## 已确认通过

- 标准扫描和完整快速快照都通过同一个 `sourceCandidate` 投影收集视频、NFO 和常见图片；生成阶段先处理全部视频，再处理附件，没有为附件追加第二次网盘目录遍历。
- 图片按 `.jpg/.jpeg/.png/.gif/.webp/.bmp` 大小写不敏感匹配，名称不限；NFO 按 `.nfo` 匹配。配置开关在扫描阶段过滤，预览仍使用只读标准抽样，不调用附件取链或内容下载。
- `qualifiedDirectory` 同时校验媒体库、账号、源文件 ID、当前播放定位码、源路径、输出根和相对路径，并读取实际 `.strm` 内容进行登记匹配；目录 ID 必须精确相同，父子目录不继承资格。生成失败、外部/修改/缺失/失配 STRM 都不会放行附件。
- 附件目标在取链前经过同目录有效 STRM、根句柄、父目录身份和叶文件检查；已有普通文件直接跳过，目录、符号链接和其他非普通文件报告冲突。发布使用同目录临时文件、设备/inode 身份、硬链接 no-replace 和再次核验，竞态不会覆盖用户文件。
- `DownloadLink` 使用 SDK 后台只读预算并复用账号间隔/冷却；内容请求不占 SDK API gate，按账号跨任务最多 2 个并发并限制等待队列。NFO/图片大小、总时限、单次时限、重试、Retry-After、可信 HTTPS 115 域、重定向、Cookie 不转发和完整凭据回执均有实现及测试。
- `sidecar_download_intents` 独立于 STRM 登记，阶段推进使用完整旧快照和 revision 条件，target 唯一；迁移 00008 的升级、非空 Down 拒绝、空表回退、并发保留和重开测试通过。恢复不重新下载，正式文件保留，只清理可证明属于本应用的临时文件。
- 条目键和分类统计区分 STRM/NFO/图片，汇总使用通用成功/跳过/失败项，不把附件计入 STRM registry；附件下载日志只记录稳定 ID、类型、字节数和错误码，不记录路径、文件名、正文、Cookie 或签名链接。前端全量同步页和执行预览已说明附属文件只同步网盘已有文件、依赖同目录有效 STRM、本地已有文件跳过且预览不下载。
- 新增和改动的手写 Go 文件均包含中文文件职责及主要方法注释；未手改生成代码。

## 本次修复

1. `backend/internal/service/strm/sidecar.go`：附件源缺少播放定位码时，先完成本地已有目标检查。这样已存在普通附件仍按合同跳过且零取链/下载，只有目标不存在时才返回定位码错误。
2. `backend/internal/service/strm/sidecar.go`：同目录 STRM 资格校验增加当前视频 `PlaybackCode` 比对，避免旧定位码的登记文件被误认为当前有效来源。
3. `backend/internal/service/strm/generate.go`：为凭据在同步期间变化增加稳定的 `provider_credential_changed` 任务错误分类。
4. `frontend/src/features/media-library/library-full-sync-page.tsx`：全量同步说明补充 NFO/图片的实际同步条件和跳过规则。
5. `backend/internal/service/strm/sidecar_test.go`：补充已有附件在源定位码缺失时仍为跳过状态的断言。

## 验证命令

- `go test ./internal/service/strm ./internal/service/provider115 ./internal/domain/sidecar ./internal/repository/sqlite`：通过。
- `go test -race ./internal/service/strm ./internal/service/provider115 ./internal/domain/sidecar ./internal/repository/sqlite`：通过。
- `make check`（backend）：通过；包含 `go vet`、全量单测和 race 门禁。
- `make lint`（backend）：通过，0 issues。
- `pnpm exec eslint src/features/media-library/library-full-sync-page.tsx`：通过。
- `pnpm exec tsc --noEmit`（frontend）：通过。
- 追加验证：`pnpm test:run --maxWorkers=1 --testTimeout=15000` 通过 15 个文件、146 个测试；受影响的媒体库详情与 STRM 管理浏览器场景通过 9/9。
- `make build smoke` 与 `make docker-build docker-smoke` 通过。带 FHS Chromium 库运行完整桌面集成时通过 33/40；7 项失败来自旧的账号初始化、健康状态和媒体库表单场景（断言依赖测试顺序/共享状态），不是本次侧车路径，真实 115 下载仍未验证。

## 仍需父任务完成的验证

- 在父任务收尾阶段执行全量前端测试、受控浏览器场景、完整集成脚本、最终 build/smoke 和 Docker build/smoke；本复核未声称真实 115 下载已验证。
- 变更应由父会话按附属同步任务独立提交；本代理未提交代码。
