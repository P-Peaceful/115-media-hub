# SDK 实际实现交接

SDK 子任务在原注释提交 61ecef8 后实现，SDK独立提交为2490d1c。实现主体：`internal/adapter/provider115`、`service/provider115/transport.go/scan.go`、`strm/scan.go/rapid_scan.go`。

- `NewWithConfig` 注入间隔/快速开关；service 懒构造并共享一个 adapter。`doRequest`、`checkCredential`、`accountScope`、`inspectBusiness` 为 provider 包内部真实复用点。Inspect 在释放账号资格前登记业务冷却。
- 交互45秒、播放15秒；后台只读 `max(45s, interval+15s)`，单次15秒。新的附件取链属于后台只读，不能直接沿用播放15秒预算导致合法60秒间隔下第二个附件必失败。真实消费者需要时增加明确下载取链操作/策略，复用 m115 编解码和可信 URL 校验，不降低播放器合同。
- `ErrCredentialChanged` 保留 ErrUpstream/503 身份，标准/快速/预览均致命停止；无论何时变更凭据，不可悄悄改用新账号。下载结束到最终发布之间仍须复核**同一凭据快照**：只比较UID不够。建议下载回执带一个不含公开凭据的校验方法/闭包，由 provider 捕获Cookie快照供发布前调用；不将Cookie/URL带入意图或日志。
- `ScanFiles` 为1150内部页，公共 `Files` 仍200；`RapidScan` 只返回完整快照。标准/快速目前只投影视频；附属应在这两个已有投影点同时保留 NFO/图片及原始大小/pc/目录ID，禁止再扫描一轮。标准 Files 已为全部非目录文件保留pc，无需修改公开FileType；NFO按后缀识别。
- 预览独立 `preview_scan.go`，标准BFS 3目录/5实际请求含重试/15秒/20样例，不接快速快照、不取下载链、不写文件。新增必需 `mode=sample` 和六个stop_reason（含sample_limit）；不要回退这些行为。
- STRM 的 `generateFile` 只有成功新写或验证同源内容后才无错返回；附属仍应在准入和发布前按登记+实际STRM内容重新验证本目录资格。不能只复用生成返回布尔值一直信任到下载完成。
- 内容下载需单独可取消共享并发2/有界队列，不能长期持有SDK的API gate或全局outputMu；长期I/O不进数据库事务。
- 下载意图独立领域/表。临时文件先O_EXCL创建并持久化身份，发布前检查仍为该文件；不确定归属不删除。恢复只清理可证明拥有暂存，正式已有文件保留，下载必须等本轮同目录有效STRM建立后才准入。

验证环境：Node24固定路径同父任务pause.md；浏览器库可用 `LD_LIBRARY_PATH=/nix/store/zwgl9ci65rjyxndijj1fw9cnn373bp50-listen1-2.33.0-fhsenv-rootfs/usr/lib64`。单测串行/15秒单场预算和浏览器1 worker/90秒适应该Nix冷启动；不要削减行为断言或加固定sleep。完整集成脚本支持 `--workers 1 --timeout-ms 90000`，在最终工作区统一执行。
