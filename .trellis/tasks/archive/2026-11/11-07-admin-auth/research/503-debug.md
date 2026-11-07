# 认证 bootstrap 返回 503 的后续记录

## 已确认原因与用户决定

运行中的数据库报告已应用版本 2，但其早期认证表结构使用整数管理员 `id`、没有 `singleton` 列，并使用 `auth_sessions`/`security_audits`，而当前结构为 `admin_sessions`/`audit_records`。公开健康检查只 ping SQLite，因此健康检查仍返回 200；管理员查询失败，bootstrap 返回 503。隔离运行的 debug/no-inline 仓储测试通过，排除了最初关于调试器/reflection 的猜测。

用户明确拒绝兼容，并自行删除旧数据库。源码不保留兼容迁移或版本 3；`sqlite.Open` 仍只加载规范的嵌入式 SQL 迁移。此代理没有修改在线数据库或其运行进程。主代理随后验证了直连/代理 bootstrap 均返回 200。用户替换数据库中现有凭据不属于本修复范围。

## 保留的修复

仓储认证读取现在提供有类型且受允许列表约束的 `storage_operation` 和 `storage_reason` 诊断，同时保留底层错误身份。HTTP 层负责状态读取的运维错误日志，记录模块/操作/结果/请求关联/耗时，不记录 SQL、原始驱动文本、密码、哈希、Cookie 或 CSRF 密钥。

调用方取消的请求记录为 DEBUG 取消结果并中止，不伪造 503。真正的存储不可用仍返回安全的 503 错误封装和 ERROR 运维记录。测试覆盖已关闭数据库的诊断，以及取消 bootstrap 时的响应/日志边界。

## 后续密码策略

用户最新批准的密码规则为 8–32 个 U+0021 至 U+007E 可打印非空格 ASCII 字符。服务初始化/登录/重置、HTTP 请求、本地隐藏输入和显式请求的 stdin 输入均统一执行。CLI 仅将可选 LF/CRLF 视为行封装，不裁剪密码空白。OpenAPI 和 Go/TypeScript 生成结构使用 minLength 8、maxLength 32 及绝对结尾 ASCII 正则，避免 ECMAScript `$` 放过末尾换行。测试覆盖接受 32、拒绝 33、接受纯字母/数字/标点，以及拒绝非 ASCII/空格/控制字符。
