# 项目内 115 客户端设计

已审阅设计的落盘版本：用户选择项目内客户端并同意实施。后续用户明确调整会员显示为上游信息，本节修订记录该要求。

## Approaches and choice
采用现有 internal/adapter/drive115 内的轻量客户端。独立 SDK 模块增加发布维护成本；保留/补丁外部 SDK 不满足移除依赖的要求。

## Boundary
初始改造集中在适配层、测试、Go 依赖和契约文档；用户会员显示调整另涉及必要的资料服务、HTTP 契约及前端显示。Adapter/BrowserAdapter、Credential、Directory/DirectoryPage 继续作为服务层边界，AccountProfile 将数字等级字段替换为可选上游名称。Challenge 内部改为本地二维码会话；API 不暴露它。登录三种设备保持一致。

## Protocol and ownership
- client.go: net/http.Client 共享连接池，超时与 NewRequestWithContext；逐请求 Cookie/Referer/统一 115Browser UA；禁止将 url.Error 或响应体放进返回错误。UA 参考 115-station 115.go，可缓存版本或采用稳定默认，同一会话一致。
- types.go: 严格的 JSON 数字/字符串归一化，ID 保留十进制文本，不经过 float64；仅接受协议预期类型。状态/大小/时间同样兼容上游两种编码。
- auth.go: 参考 115.go 的 token/image/status/result URL 和表单；原始 uid/time/sign 仅保留内存；未知状态/不完整 Cookie 为错误；轮询失败不得伪装 waiting。
- profile.go: nav 身份信息、files 根目录验证、index_info 可选容量和设备信息。会员名称从可选 vip_limit 查询取得原文；不根据数字 vip 推导状态或名称，不把缺失资料补成零。
- browse.go: 参考 panpath.go/files115.go，以 /files 响应 path 作为根至当前目录链；根用本地“我的网盘”显示；最后一级与请求 cid 一致，aid 不正常则拒绝。列表继续使用当前分页/筛选参数和条目字段语义。列表有 path 时同样校验身份。
- errors.go: 项目自有错误，无 SDK 错误依赖；仅提取并脱敏白名单 code/message。HTTP 非成功不作为业务成功。共享 request 函数不依赖所有端点相同 envelope。

## Deliberate adaptations
参考接口与响应协议，不复制站点 Handler/数据库/日志。保持正常 TLS 校验，不引入其明文 HTTP 镜像或证书豁免。不增加持久化缓存。镜像回退只在已研究 HTTPS 地址、有界且不掩盖登录失效时采用。读操作可用有界节流，取消等待必须响应 context；登录完成不自动重试。

## Compatibility and rollback
现有保存凭据继续可用；会员资料 API 用可选 membershipName 替代 membershipLevel，其他登录/浏览契约不变。还原本任务文件和模块依赖即可回滚；不更改数据库、不删除既有任务或用户改动。风险：Cookie 接口无稳定公开契约；真实设备扫码仍需用户账号验收。

## User-directed membership revision

Add optional Cookie vip_limit(feature=2) enrichment using data.user_limit.level_name; propagate verbatim as membershipName, replace public numeric membershipLevel. UI uses name or “会员信息未知”, no numeric/status-based label fallback. Keep coarse membershipStatus only for existing contract compatibility; do not infer it from label text or numeric levels. Legacy snapshots without membershipName must remain unknown in display; remove numeric legacy normalization. Persist only selected safe account fields. Generate Go/TS contracts from OpenAPI and cover backend, snapshots and both UI surfaces. This explicitly expands the original adapter-only boundary to necessary service/API/generated/frontend files; no other UI redesign.
