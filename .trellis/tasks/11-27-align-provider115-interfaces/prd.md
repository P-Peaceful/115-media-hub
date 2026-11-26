# 对齐115-station网盘接口

## Goal

让项目的 115 网盘下载 SDK 与 115-station 使用同类的接口通道、参数协议和 CDN 请求头处理，优先解决附属文件取链阶段的 `sidecar_take_link_failed`，并保持播放、目录扫描和现有限流安全边界不回退。

## Background and confirmed facts

- 当前项目直接依赖 `github.com/SheltonZhu/115driver v1.3.5`，但只使用其 `m115` 加解密包；实际取链固定调用 `https://proapi.115.com/app/chrome/downurl`。
- 当前下载取链请求发送 `{"pickcode": ...}`，并要求响应按源文件 ID 索引且回传 `pick_code` 能通过核验。
- 当前目录扫描已使用 `webapi.115.com/files`、`category/get` 和快速扫描的 `app/chrome/downfolders`，与 115-station 的 Cookie 目录通道基本一致；本任务不为“接口名称不同”而替换这些已验证的扫描接口。
- 115-station 的 Cookie 下载通道优先尝试 `proapi.115.com/android/2.0/ufile/download` 和 `pro.api.115.com/android/2.0/ufile/download`，请求字段为 `pick_code`；失败后回退到 `webapi.115.com/files/download` 的加密 POST，再回退到带 `pickcode` 查询参数的 GET。
- 115-station 会保存取链响应的 `Set-Cookie`，并在 CDN 403 时按签发 UA、响应 Cookie、登录 Cookie的组合进行有限重试；当前项目内容请求不携带 Cookie，也不保留取链响应 Cookie。
- 当前错误发生在 CDN 请求之前，因此首先需要修复取链通道覆盖；CDN 头兼容性作为同一下载 SDK 改造的一部分补齐。

## Requirements

### R1. 统一下载接口矩阵

- 为 Cookie 通道集中定义并校验下载接口地址和协议类型，至少覆盖 Android 双端点、现有 Chrome 兼容端点、WebAPI 加密 POST 和 WebAPI GET。
- Android 请求使用 `pick_code`，Chrome 兼容请求使用现有 `pickcode` 协议，WebAPI POST/GET 按各自返回结构解析；不得把不同协议的字段或响应解析器混用。
- 取链结果统一为内部下载目标，除 URL 外保留签发 UA 和取链响应下发的 Cookie；不向日志、API 响应或任务错误暴露签名地址、Cookie 或 pickcode。

### R2. 多通道回退和错误分类

- 附属文件下载使用与 115-station 等价的回退顺序：Android 双端点 → WebAPI 加密 POST → WebAPI GET；现有 Chrome 接口作为兼容回退，具体顺序在设计文档中固定并可测试。
- 播放解析和后台附件下载共享协议解析与目标校验，但保留各自超时、优先级、并发和日志语义。
- 认证失败、文件不存在、限流、响应协议错误和所有通道耗尽必须保持稳定的 `errors.Is` 身份及现有安全错误码；不得把已识别的授权/限流错误重新包装成普通取链失败。

### R3. CDN 下载兼容性

- 使用取链时的同一 User-Agent；按 115-station 的策略保存并有限组合尝试响应 Cookie 与登录 Cookie。
- 仅对明确可重试的 CDN 403 或临时状态重试；保持当前 HTTPS、115 域名边界、重定向跳数、大小、长度、超时和凭据变更检查。
- 内容下载仍受账号级并发/等待槽和现有 115 API 冷却控制，不能通过新增回退通道绕过限流或请求预算。

### R4. 目录接口保持兼容

- 对现有目录、元数据和快速扫描地址做接口矩阵测试/注释确认；没有 115-station 实际差异的接口不改行为。

### R5. 注释、测试和回归

- 每个变更的 Go 文件保留/补充中文文件职责和主要方法职责注释。
- 为每个取链通道、字段协议、回退顺序、Set-Cookie 透传、403 重试、授权/限流错误和凭据脱敏增加单元测试。
- 通过后端完整质量门禁：格式、vet、测试、race、lint；记录真实执行结果。

## Acceptance Criteria

- [x] 使用同一个有效 pickcode 时，Android 双端点成功、Android 失败后 WebAPI POST 成功、POST 失败后 GET 成功的场景均能解析出受信任下载目标。
- [x] 115-station 风格的 `pick_code` 单 URL 响应与当前 Chrome 风格的按文件 ID 响应都能被正确解析，并拒绝源文件/定位码不匹配的响应。
- [x] 取链响应的 Set-Cookie 被用于 CDN 请求组合；403 按有限次数重试，非 403 不进行无意义重试；成功下载不泄漏凭据。
- [x] 附属文件的取链失败日志不再把已识别的授权、限流、文件不存在和协议错误统一归为 `sidecar_take_link_failed`。
- [x] 目录浏览、快速扫描、播放和已有 STRM 生成测试全部保持通过；未改变既有目录分页、账号配额和安全校验合同。
- [x] `make -C backend check` 通过，且变更文件满足中文注释规范。

## Out of scope

- 不在本任务内新增 115 OpenAPI OAuth 登录、Token 刷新或 OpenAPI 账号配置；仅为未来接入保留可扩展的下载通道边界。
- 不重写目录扫描算法、STRM 文件格式、媒体库附属文件匹配规则或前端接口。
- 不将 pickcode 写入 STRM 内容，也不放宽 Cookie、签名 URL、CDN 域名和重定向安全限制。

## Open questions

无阻塞性问题；Chrome 兼容回退的具体位置由设计文档固定，并以现有播放回归测试为准。
