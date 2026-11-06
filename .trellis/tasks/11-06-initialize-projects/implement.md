# 实施计划

## 审批门槛
用户已通过“开始”批准实施，任务已进入 in_progress。

## 执行步骤
- [x] 阅读适用的前后端规范与对应依赖的准确版本文档；确认运行时版本和 Docker 可用性。
- [x] 固定兼容的工具链、依赖和生成器版本，定义共享健康检查 OpenAPI 契约。
- [x] 实现 backend/ 的配置、SQLite/goose 初始化、健康检查、结构化日志、优雅关闭及有效测试。
- [x] 实现 frontend/ 的 Vite 应用、providers、类型化健康请求和无障碍初始页面，并添加行为测试与浏览器冒烟流程。
- [x] 添加可重复执行的契约生成、开发与生产编排命令、go:embed 集成和多阶段 Docker 配置。
- [x] 添加格式、lint、类型、测试、构建与 CI 检查；编写环境配置和启动文档。
- [x] 执行 Trellis check 审查，修复发现的问题，同步实际目录和实现规范，记录验证证据。

## 验证方式
在 frontend/ 中执行：pnpm 冻结锁文件安装、format:check、lint、typecheck、test:run、build，以及覆盖初始流程的 test:e2e。

在 backend/ 中执行：格式校验、go vet ./...、CGO_ENABLED=1 go test ./...、CGO_ENABLED=1 go test -race ./...，以及固定版本的 golangci-lint run ./...。

集成验证：重新生成契约并检查差异；构建嵌入前端的生产二进制；冒烟验证健康检查、SPA、错误路径、首次及重复数据库启动和优雅关闭；Docker 可用时构建并验证 CGO 容器。

记录实际命令结果和环境限制；跳过的检查不得标记为通过。

## 风险与审查点
验证固定版本的 HeroUI v3 API，以及 React/Tailwind 兼容性。确保迁移和前端资源位于各自 Go 包内可嵌入的路径。后端开发启动不依赖预先构建 SPA。重点审查 API/静态资源回退和请求日志。新增文件与规范路径调整可回退，不修改无关开发工具。

## 验证记录
具体命令结果与环境说明见 [validation.md](./validation.md)。
