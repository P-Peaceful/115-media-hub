# 执行计划：对齐 115-station 网盘接口

## 实施顺序

1. 在 provider115 SDK 中集中定义下载端点、下载目标和协议解析边界；扩展适配器端点白名单与操作策略。
2. 实现 Android 双端点取链及单 URL 响应解析，保留 Chrome 现有解析器作为兼容通道。
3. 实现 WebAPI 加密 POST/GET 回退，统一错误码优先级和来源身份校验。
4. 让播放和 sidecar 调用共享下载目标解析；sidecar 继续使用后台队列、单文件预算和凭据监视。
5. 透传签发 UA/Set-Cookie，补齐 CDN 403 组合重试，同时维持可信跳转、长度和大小检查。
6. 补充端点顺序、字段协议、响应形态、回退、错误分类、限流和凭据脱敏测试；更新变更 Go 文件中文注释。
7. 运行格式检查、vet、全量测试、race、lint 和 `make -C backend check`，修复回归后再做最终检查。

## 主要文件范围

- `backend/internal/service/provider115/playback.go`：下载目标解析、协议解析和来源身份核验。
- `backend/internal/service/provider115/download.go`：目标头透传和 CDN 下载组合重试。
- `backend/internal/service/provider115/service.go`：默认端点/服务配置。
- `backend/internal/service/provider115/transport.go`：请求头、业务错误与适配器调用边界。
- `backend/internal/adapter/provider115/client.go`、`types.go`：端点白名单和下载操作预算。
- 对应 `*_test.go`：各端点和回归场景。

## 验证与回滚点

- 回滚点一：仅新增协议解析测试和端点常量，原 Chrome 流程仍可运行。
- 回滚点二：启用 Android/Web 回退并验证现有播放/sidecar 测试。
- 回滚点三：启用 CDN Cookie 组合并验证 403、凭据变更和日志脱敏。
- 失败时保留原错误身份和调用方边界，不用放宽端点域名或关闭限流来“修复”测试。

## 计划命令

```text
gofmt -w <changed-go-files>
go vet ./...
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
golangci-lint run ./...
make -C backend check
```

## 实际验证结果

- `make -C backend check`：通过，包含 `gofmt`、`go vet ./...`、全量测试、`go test -race ./...` 和 `golangci-lint`，lint 为 0 issues。
- `git diff --check`：通过。
- 已通过受控 RoundTripper 验证五通道顺序、每通道单次发送、账号间隔、加密载荷、Set-Cookie、CDN Cookie 组合、跨主机清理、播放 `f=3` 拒绝和日志脱敏。
- 未使用真实 115 账号联调；真实私有接口的在线可用性仍需部署环境中以用户账号验证。
