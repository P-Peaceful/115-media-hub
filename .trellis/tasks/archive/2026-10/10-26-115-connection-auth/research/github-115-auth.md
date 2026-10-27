# GitHub 115 扫码授权实现调研

调研日期：2026-10-26

## 结论摘要

若目标是尽快为文件浏览、STRM 和 302 播放建立可工作的自托管连接，现有项目中最成熟的是 Cookie 扫码路线：后端创建二维码会话、前端展示二维码、后端轮询状态，用户确认后换取 Cookie 并验证账号。推荐优先参考 MIT 许可的 Go 项目 `SheltonZhu/115driver` 的接口边界，并用项目自己的 adapter 隔离依赖。

官方 115 开放平台 OAuth 更标准，凭据是 access token 与 refresh token，但自建应用需要申请 AppID/密钥；公开项目常借用已有应用或公共回调服务，这不适合作为本项目默认依赖。OpenList 文档截至调研时仍将“手机扫码授权 PKCE 模式”标记为未实现。

## 参考项目

### SheltonZhu/115driver

- 地址：https://github.com/SheltonZhu/115driver
- 语言/许可：Go，MIT。
- 适配度：与本项目后端语言一致，提供 `QRCodeStart`、`QRCodeStatus`、`QRCodeLoginWithApp`、`CookieCheck`、`LoginCheck` 和账号信息能力。
- 状态：`0=waiting`、`1=scanned`、`2=allowed`、`-1=expired`、`-2=canceled`。
- 凭据：`UID`、`CID`、`SEID`、`KID`；可输出 Cookie 字符串，但本项目不应把它返回前端或写明文配置。
- 设备类型：web、android、ios、tv、alipaymini、wechatmini、qandroid；桌面设备类型在代码中禁用。
- 注意：SDK 的 `LoginCheck` 注释说明可能影响其他设备登录；连接验证应优先研究使用不会登出其他设备的 `CookieCheck`，再调用读取型账号/根目录接口获得能力证据。
- 集成建议：若采用依赖，固定发布版本，包在 `internal/adapter/drive115` 内，不让 SDK 类型越过 adapter 边界；为 HTTP 超时、错误归类、限流和未来替换保留项目接口。

### lingwunb666/115-MCP-Server

- 地址：https://github.com/lingwunb666/115-MCP-Server
- 语言：Python，封装 `p115client`。
- 可借鉴的产品接口：将扫码拆成 `start_qrcode_login`、`get_qrcode_login_status`、`finish_qrcode_login` 三步；返回 session ID、二维码 URL、规范化状态，完成后才切换活动凭据。
- 可借鉴的状态：waiting、scanned、signed_in、expired、canceled。
- 不直接复用：它把 Cookie 保存到用户指定文本文件；本项目将 Cookie 保存在服务端 SQLite；本任务的后续用户决定移除了凭据加密。

### cdlongbow/115StrmTool

- 地址：https://github.com/cdlongbow/115StrmTool
- 语言/许可：Python，GPL-3.0。
- 可借鉴的 UX：管理页面本地生成/展示二维码，成功后自动填充连接；默认推荐支付宝或微信小程序，明确提醒 Web 端更容易触发 IP 登录异常或挤掉已有登录。
- 许可约束：只能作为行为和风险证据，不能复制其 GPL 实现到本项目的 Apache-2.0 代码中。

### OpenList 与 OpenList 文档

- 地址：https://github.com/OpenListTeam/OpenList
- 115 文档：https://github.com/OpenListTeam/OpenList-Docs/blob/main/pages/guide/drivers/115.md
- 115 Open 文档：https://github.com/OpenListTeam/OpenList-Docs/blob/main/pages/guide/drivers/115_open.md
- 许可：核心与文档为 AGPL-3.0。
- Cookie 扫码流程与 `115driver` 一致，并提示优先选择不常用的设备类型，以免挤掉现有会话。
- 官方开放平台驱动保存 access token/refresh token，并在本地刷新；文档提示 refresh token 轮换与 IP 限频，同一应用多次授权会使较早的令牌失效。
- 许可约束：不复制代码；只用于交叉验证协议行为、错误场景与设备选择风险。

### ChenyangGao/p115client

- 地址：https://github.com/ChenyangGao/p115client
- 语言/许可：Python，MIT。
- 提供 Cookie 客户端和开放平台客户端；文档说明 refresh token 单次使用，刷新请求已被服务端接受但响应读取失败时可能导致令牌报废，因此刷新必须按不可重复副作用处理，失败后可能要求重新授权。
- 官方 OAuth 示例需要 AppID/回调，或借用其他项目的应用与授权服务。后者不应成为本项目的默认生产方案。

## Cookie 扫码协议形态

1. 后端请求二维码 token，得到 `uid`、`time`、`sign` 和二维码内容。
2. 后端创建短期本地扫码会话，只把本地 session ID、二维码内容和过期时间返回给前端。
3. 后端用 `uid/time/sign` 查询 115 状态，并归一化为 waiting、scanned、allowed、expired、canceled 或 failed。
4. allowed 后，后端选择绑定设备类型并以 `uid` 换取 `UID/CID/SEID/KID`。
5. 后端验证 Cookie、读取账号身份和根目录能力。
6. 后端将验证后的凭据 JSON 保存到稳定连接；临时扫码参数随后从内存删除。

二维码签名和 Cookie 都不应由浏览器持有。前端轮询本项目 API，而不是直接请求 `*.115.com`。

## 设计风险

- Cookie API 并非稳定的公开 OAuth 契约，115 可改变端点、响应或风控策略；adapter 必须独立、错误需归类、UI 要支持重新授权。
- 设备类型会影响已有登录；默认 `alipaymini` 比 web 更适合作为自托管工具设备，但仍应在 UI 中说明影响。
- 二维码会话参数可用于完成登录，应按短期秘密处理：仅内存保存、限制管理员会话、设过期时间、完成后删除、禁止日志记录。
- 网络错误不能自动判定 Cookie 失效；验证结果需要区分 invalid、limited、unreachable 和 unknown。
- 按用户最终决定，凭据明文保存在本地数据库；数据库和备份因此需要按敏感登录凭据保护。
- 第三方库可减少协议维护成本，但不能取代项目自己的 adapter、超时、限流、错误模型和测试替身。

## 路线比较

| 路线 | 优点 | 代价与风险 | 适合首版 |
| --- | --- | --- | --- |
| Cookie 扫码 + 自有 adapter | 不要求开发者应用；GitHub 实现成熟；覆盖现有私有文件 API | 非官方契约；有设备挤下线、风控和 Cookie 失效风险 | 推荐 |
| 官方 OAuth + 自有 AppID | 官方 token 模型；可刷新和撤销；长期边界更清晰 | 需要申请应用并维护回调/PKCE；可用 API 需逐项验证 | 适合后续或有应用信息时首选 |
| 借用第三方 AppID/回调服务 | 用户无需自行申请 | 凭据流经或依赖第三方；可用性、信任和配额不可控 | 不采用 |

## 建议的首版决策

采用 Cookie 扫码，首版支持 `web`、`alipaymini`、`wechatmini` 三种设备方式并默认选择 `web`；Web 方式需要显示可能影响已有网页登录或触发 IP 异常的提示。使用 `115driver` 的 MIT Go 实现作为优先依赖候选，但通过项目 adapter 封装并在设计阶段确认版本与最小调用面。官方 OAuth 作为后续可替换凭据类型保留，不与首版同时实现。
