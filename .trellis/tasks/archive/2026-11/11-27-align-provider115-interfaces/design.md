# 技术设计：对齐 115-station 下载接口

## 1. 边界

在 `backend/internal/service/provider115` 与 `backend/internal/adapter/provider115` 内增加统一的下载目标解析能力。服务层负责账号凭据、业务错误、来源身份和 sidecar 生命周期；适配器负责端点白名单、请求预算、冷却、重试和脱敏错误。目录扫描现有接口保持不变，仅补充矩阵测试和中文说明。

## 2. 下载数据流

```text
DownloadSidecar / ResolvePlayback
  → DownloadResolver（按账号和操作类型进入受控适配器）
  → Android proapi
  → Android pro.api
  → 现有 Chrome downurl 兼容端点
  → webapi files/download POST
  → webapi files/download GET
  → DownloadTarget{URL, User-Agent, Set-Cookie}
  → CDN 内容请求（可信域/重定向校验、有限 UA/Cookie 组合）
```

Android 接口使用加密载荷 `{"pick_code": pickcode}` 并解析解密后的单 URL；Chrome 接口保留现有 `{"pickcode": pickcode}` 和按源 ID 的响应校验；WebAPI POST 使用加密 `{"pickcode": pickcode}` 并解析 `file_url`，GET 兼容多种 `url`/`data.url` 响应形态。

## 3. 回退、错误和预算

- 取链回退只在协议不可用、临时网络失败或明确的接口不支持时继续；认证、文件不存在、限流、取消、凭据变更和不可信目标立即结束。
- 每个端点请求必须经过现有账号级适配器，沿用后台 `DownloadLink` 操作、配置间隔、最大响应体和实际发送预算；不允许直接 `http.Client.Do` 绕过调度。
- 解析器统一返回既有领域错误身份；未知通道耗尽错误保留 `ErrDownloadTakeLink`，已识别错误不得再次包装遮蔽。
- 下载目标对象只在内存中携带头信息；日志只记录稳定错误码，不记录 URL、Cookie、UA、pickcode 或响应原文。

## 4. CDN 请求策略

取链响应收集 `Set-Cookie`，目标 UA 取响应实际签发 UA，缺失时使用固定 115Browser UA。内容请求按有界顺序尝试：响应 Cookie、无 Cookie、响应 Cookie+登录 Cookie、登录 Cookie；登录 Cookie 只发往 `115.com` 账号域，`115cdn.*` 只接受域名匹配的取链响应 Cookie。只对 403 继续下一组合，其余状态按现有内容错误/过期/限流分类。每次重定向重新检查可信域并重设受控头，跨主机后清除 Cookie。

## 5. 兼容和回滚

保留 Chrome 端点与原有响应解析作为兼容回退，避免现有播放链路因接口切换立即中断。新解析器以独立测试替身验证，若线上通道异常可通过配置/代码回退到原 Chrome 路径；不改数据库和公开 API，因此回滚仅涉及服务/适配器代码。

## 6. 风险

- 115 私有接口可能按账号、UA 或节点返回不同响应；测试必须覆盖单 URL、按文件 ID、错误信封和空 data。
- 多通道会增加潜在请求次数，必须与现有请求间隔和后台总预算绑定，并在日志中区分“回退”与“重试”。
- 登录 Cookie 仅用于 CDN 兼容重试，不能进入 STRM、任务错误或日志。
