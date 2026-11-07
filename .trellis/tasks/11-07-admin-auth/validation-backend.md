# 后端实现证据

## 范围与分层

已实现规范认证 OpenAPI 契约并重新生成 Go/TypeScript 两端产物；新增领域包、认证策略服务、真实 SQLite 仓储/凭据迁移、HTTP 传输层/默认管理路由保护、应用组装和恢复 CLI。未引入服务提供商授权代码。当前服务器输出默认值为 `/app/media`；初始化只保存经过语法校验的配置，不创建文件。Docker 数据目录统一位于 `/app` 下。

密码使用锁定的 bcrypt v0.58.0，生产成本为 12，限制为 8–32 个可打印非空格 ASCII 字符（U+0021–U+007E），不裁剪且不强制字符类别组合。浏览器凭据由 32 个密码学安全随机字节编码为 64 位十六进制字符；只持久化 SHA256 查询哈希。CSRF 证明派生自 Cookie，并以哈希绑定持久化；签名的预认证 Cookie 有效期 20 分钟，使用重启后更换的随机密钥。Bootstrap 刷新会复用有效预认证绑定。已认证会话在固定数据库有效期内可跨重启存活。必要审计写入失败会回滚变更，并输出脱敏的 `audit_unavailable` 运维结果。

登录完成密码验证后，会在事务中校验预期密码哈希，避免密码恢复后正在进行的旧密码登录重新创建会话。限流器限制可信对端的请求窗口、总尝试次数、昂贵操作并发数和保留的对端状态；任意转发头不能更改对端身份。未来注册的管理路由默认要求会话，状态变更请求默认要求 CSRF。未知 API 路由仍返回 JSON 404。健康检查/SPA/静态资源命名空间行为保持独立。

## 命令与结果

- `make generate`：通过；生成 Go 和前端类型。
- `cd backend && go test ./...`：完整实现及最终 8–32 可打印 ASCII 密码策略下通过。
- `cd backend && make check`：最终 ASCII 密码、安全存储诊断和 bootstrap 取消处理修改后通过：gofmt/goimports 检查、vet、全部测试、竞态测试和 golangci-lint（`0 issues`）。
- 真实 SQLite 测试覆盖单例初始化竞争、所有必须审计的回滚边界、重启/会话持久化、恢复时撤销会话/保留配置、仅应用表数据库升级和拒绝不可逆降级。
- 确定性同步测试覆盖旧密码登录与已提交重置的竞争，不使用 sleep。
- HTTP 测试覆盖初始化/登录/登出、匿名/认证 bootstrap、缺失/错误 CSRF、跨源及伪造转发头、固定过期、Cookie 轮换/持久化/HttpOnly/SameSite/path、配置 HTTPS 与直连 TLS Secure Cookie、有界 JSON 及必填/未知字段、安全错误封装、限流/Retry-After、注册路由默认保护、公开健康/未知 API 和不含密钥的运维日志。
- 服务测试覆盖令牌生成/往返、CSRF 轮换和匿名过期、固定会话过期、有界准入与时钟恢复、无效设置、8–32 字符可打印 ASCII 密码，以及结构化/脱敏结果日志。
- CLI 测试覆盖显式管道输入、stdin 非终端时要求隐藏终端输入、32 字符 ASCII 边界、拒绝空白/非 ASCII/控制/多行值，以及可选 LF/CRLF 行尾处理。

## 受支持 API 核验

查阅官方 GORM 事务文档（https://gorm.io/docs/transactions.html），其中仍支持传统 API 和 Generics API；保留锁定的 v1.31.2 传统 `WithContext`、`Transaction`、`Take`、`Create`、`Updates`、`Delete` 方法，并检查错误/RowsAffected。查阅 Gin v1.12.0 官方文档（https://pkg.go.dev/github.com/gin-gonic/gin@v1.12.0），包括 `SetTrustedProxies(nil)`。查阅标准库 Cookie/随机数文档（https://pkg.go.dev/net/http#Cookie、https://pkg.go.dev/crypto/rand）及安装的 Go 1.26.9 文档/源码：`rand.Read` 会填满缓冲区且不返回错误；`rand.Text` 保证至少 128 位熵，但不承诺固定长度。因此令牌不依赖它的长度。使用标准 `http.SetCookie`，不使用已弃用生态辅助函数。

通过 `go doc golang.org/x/crypto/bcrypt.GenerateFromPassword`、`CompareHashAndPassword` 和 `golang.org/x/term.ReadPassword` 查阅锁定安装包文档；这些 API 均未弃用。Bcrypt 库另有 72 字节上限；应用限制更严格，为 8–32 个可打印非空格 ASCII 字符。终端 `ReadPassword` 是受支持的无回显输入方法。为终端输入新增并锁定 x/term v0.47.0；直接使用已有 x/crypto v0.58.0。没有无必要升级。Web connector 无法读取版本化的 x/crypto/x/term pkg.go.dev 地址，因此以本地安装版本文档/源码为准。

## 部署/恢复交接

HTTPS 在反向代理处终止时必须设置 `MEDIAHUB_COOKIE_SECURE=true`。直连 TLS 也会设置 Secure；忽略转发头。所有 Cookie 均为 HttpOnly、SameSite=Lax 和 Path=/；记住登录决定采用 Max-Age/Expires 还是浏览器会话持久性，两者均受服务端固定过期时间约束。Cookie 名为 `mediahub_session` 和 `mediahub_preauth`；所有凭据响应均为 no-store。

停止服务器并配置其现有 `MEDIAHUB_DATABASE_PATH`，然后执行 `mediahub admin reset-password` 以交互式隐藏输入/确认密码，或显式运行 `mediahub admin reset-password --password-stdin` 并通过 stdin 传入一行密码。命令不接受密码参数/环境变量。CLI 拒绝不存在/未初始化的数据库，通过常规迁移边界操作，并在同一事务中更新密码哈希/撤销所有会话/记录本地维护审计，同时保留设置。主代理负责实际二进制/容器执行文档和完整生产集成。凭据迁移的 Down 操作明确失败；应恢复兼容的二进制/数据库备份，而不是删除凭据。

## 密码/取消处理最终后续

最终密码约束统一适用于初始化、登录和重置：仅允许 U+0021..U+007E 中的 8–32 个字符，不要求组合类别。服务、HTTP 和 CLI 测试覆盖接受 32、拒绝 33、接受纯字母/数字/标点，以及拒绝中文/空格/tab/换行/DEL。stdin 最多读取 35 字节（32 字节 + CRLF + 一个溢出检测字节）；可选 LF/CRLF 仅视为行结束，不接受单独 CR，不裁剪密码空白。规范正则为 `^[!-~]{8,32}(?![\s\S])`；使用实际 Node ECMAScript 引擎验证两处 OpenAPI 正则拒绝尾随换行。生成产物一致。

保留的存储诊断只暴露允许列表中的操作/原因及请求关联；日志和客户端响应不含原始 SQL/驱动错误。已取消的 bootstrap 输出 DEBUG 取消结果，不伪造 503。新增回归和后端完整 `make check` 均通过。仓库中不再包含迁移 3/兼容表/兼容代码；旧数据库由主代理/用户明确处理。此实现者未操作在线凭据、数据库或进程。
