# 115 客户端与上游参考证据

研究日期：2026-11-25。当前项目基线提交：85e4d03。

依赖核对：backend/go.mod 已固定 github.com/SheltonZhu/115driver v1.3.5，产品源码仅引用其 pkg/crypto/m115（backend/internal/service/provider115/playback.go:14）；目录、账号和扫码不通过该 driver 实现。本轮保留编解码依赖，不引入完整 driver 客户端。

## 本项目

| 位置 | 已实现行为与改造约束 |
| --- | --- |
| backend/internal/service/provider115/service.go:63 | New 构造 Cookie/扫码相关端点及 HTTP 客户端；无主动节流。 |
| backend/internal/service/provider115/files.go:64 | 公共 FileQuery 校验与分页绑定；limit 1–200；身份/路径/总数和账号变化校验。 |
| backend/internal/service/provider115/details.go:289 | accountRequest GET，15 秒 timeout，Cookie/UA/Referer、禁重定向、响应有界；非 200 折叠为 ErrUpstream。 |
| backend/internal/service/provider115/playback.go:59 | 播放取链 POST 编码，保留播放器 UA，识别 429/401，响应大小和可信目标受限。请求不是网盘写操作，不能仅按 HTTP 方法判断业务读写。 |
| backend/internal/service/strm/scan.go:101 | 逐目录串行 200 条 all 类型扫描；按配置排除和递归；每个目录完整后才采用其结果。 |
| backend/internal/service/strm/preview.go:56 | 预览调用相同 scan，正式任务独立重新扫描；预览有期限和部分结果语义。 |
| backend/internal/service/strm/types.go:38 | STRM 消费窄 Provider 接口：Files、ResolvePlayback。 |
| backend/internal/service/strm/generate.go:90 | 先扫描再写文件；必须保持现有所有权、凭证和恢复机制。 |

公共浏览分页上限与扫描内部批次是不同合同：不应为了内部 1150 分页而无意放宽 HTTP FileQuery。快速扫描是否暴露为内部枚举方法、结果如何投影到原有扫描合同，在范围确认后形成设计。

## 115-station 固定参考

仓库：https://github.com/pancras-loe/115-station

固定提交：d4251d0083bf344b38569da7bbb4def1893b6401，日期 2026-11-24。

- [fast115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/fast115.go)：Cookie 递归文件列表 show_dir=0，1150/页；downfolders 5000/页；父 ID 与目录表重建路径，去重及完整性检查；失败回滚半成品并降级。OpenAPI 不走此快速模式。
- [files115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/files115.go)：标准目录遍历、统一 Cookie GET、节流前后锚点和目录身份检查。包含多 origin 回退，不能原样采用其中 HTTP 明文通道。
- [ratelimit.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/ratelimit.go)：默认读 1 秒，写不少于 3 秒；读间隔被 readGap 截到最多 1 秒，与文档建议调大读间隔不一致。包级节流锁不覆盖网络请求全程，不应将其当作严格并发串行保证。
- [full115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/full115.go)：队列提交、按库收清单、STRM 写入与台账、附属文件下载、完整清单才可标失效。
- [useStrmRuns.ts](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/webui/src/pages/strm/useStrmRuns.ts)：确认阶段读配置和上次扫描记录，不先进行完整网盘预览。本项目是否改变预览新鲜度需要单独决策。
- [open115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/open115.go)：OAuth/Bearer 与 /open/ufile/files 1000 分页；不是现有 Cookie 合同的内部替换。
- [incr115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/incr115.go) 与 [life115.go](https://github.com/pancras-loe/115-station/blob/d4251d0083bf344b38569da7bbb4def1893b6401/internal/api/life115.go)：事件游标去重、2 秒冷却、失败最多 3 次尝试/间隔 30 秒；增量一致性和后台生命周期属于新功能范围。

## 设计时必须处理的差异

1. 上游用于研究，不直接复制实现；保留本项目标准 TLS 验证、HTTPS、窄接口和依赖注入。
2. 节流/退避服从 context；可注入时钟/等待器验证取消及上限，不能用真实长 sleep 隐藏竞态。
3. 账号切换使旧响应失效；账号范围的限流状态不得混用凭据或泄漏凭据值。
4. 重试仅适用于明确可重试操作；限流应先冷却，不能通过立即降级/换端点放大请求。
5. 快速返回清单仍要验证目标身份、目录祖先链、排除/递归规则、去重和完整性。无法确认时保留已有结果/产物安全边界。
6. 公共目录浏览的 page/cursor、HTTP 错误码和存量播放 URL 不因内部重构变化。
7. 目录列表页成功日志保留 DEBUG 聚合策略；区分限流、取消、授权失败，禁止写入 Cookie、pickcode 或签名 URL。

## 当前规划状态

用户已选择 A：现有 Cookie 请求底层、限速/重试、快速扫描与标准降级；不引入 OAuth 或事件增量。设计与执行计划已形成，待最终摘要批准后进入实现，不声称已在用户真实网盘验证非公开接口。

## 目录 pickcode 获取补充

固定依赖源码 pkg/driver/api.go 的 ApiFileStat 为 https://webapi.115.com/category/get，op.go 的 Stat(fileID) 以 cid 查询，response.go 的 FileStatResponse 含 pick_code、file_category、paths 等字段。这为从服务端获取目录真实 pickcode 提供研究依据，无需复制上游 pickcode115.go 的本地替换表算法。设计采用只读元数据获取并核对目录身份；缺失可验证身份或 pickcode 时回退标准扫描。此处字段可能变化，实际响应仍需严格解析及失败用例验证。
