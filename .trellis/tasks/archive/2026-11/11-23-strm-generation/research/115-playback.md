# 115 扫码授权与 302 播放

现有支付宝小程序扫码得到的 Cookie 可用于 115 浏览和 Chrome 下载接口。目录响应的文件条目提供 `fid` 与 `pc`。Cookie 的 `UID` 在首个下划线前保存数字账号 ID，可与账号资料接口的 `user_id` 对照。登记仅使用身份部分，播放时重新验证当前账号身份和本次请求使用的凭据。同一账号重扫后可继续解析原有产物。 [账号 ID 的参考实现](https://github.com/ChenyangGao/p115client/blob/main/p115client/client.py#L532-L540)

下载链接签发使用 `POST https://proapi.115.com/app/chrome/downurl`。它接受 m115 编码的提取码，解码后按源文件 ID 选择返回项。项目固定使用 `115driver v1.3.5` 的 m115 编解码包，HTTP 请求、凭据生命周期、超时、响应大小和域名验证由本项目控制。签发请求使用播放器的 User-Agent，以匹配 115 CDN 可能执行的请求头校验。 [下载协议实现](https://github.com/SheltonZhu/115driver/blob/v1.3.5/pkg/driver/download.go#L64-L105) · [OpenList Cookie 驱动](https://github.com/OpenListTeam/OpenList/blob/main/drivers/115/driver.go#L61-L74)

2026-11-23 使用本地已有授权和一个媒体库样本验证。账号资料通过，列表中 UID 解析结果与资料的账号 ID 一致，且样本包含有效的提取码。115 Chrome 下载接口返回 HTTP 200，解码后记录的提取码一致，临时链接指向 `115cdn.net` 的 HTTPS 地址。使用同一 User-Agent 的 HTTP 客户端从本地临时服务跟随一次 302，携带 64 KiB Range 且无需 115 Cookie。CDN 返回 HTTP 206 和 `Content-Range`，取得 65,536 字节并识别媒体容器签名。FFmpeg 8.1.2 使用自身 User-Agent 和受限探测参数，从调用真实适配器的本地临时服务跟随一次 302，无 Cookie 解码首帧成功。两次验证均开启 TLS 校验，原数据库以只读方式打开。

模拟上游覆盖登录失效、源文件缺失、限流、畸形密文、超长响应、上游跳转、账号不匹配、可信域名边界和日志脱敏。该 Cookie 接口属于实际服务行为，115 开放平台提供单独的应用授权及下载和播放接口。 [115 开放平台](https://open.115.com/)
