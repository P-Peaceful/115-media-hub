# 初始化验证记录

## 工具链与依赖
- Go 1.26.9（模块最低版本 1.26.0）、Node.js 24.13.0、pnpm 11.28.5。
- React 19.3.1、Vite 8.3.5、TypeScript 5.9.3、HeroUI 3.2.7、Tailwind CSS 4.3.4、React Router 8.5.1、TanStack Query 5.106.0。
- Gin 1.12.0、GORM 1.31.2、SQLite 驱动 1.6.0、goose 3.29.0、lumberjack 2.2.1。
- oapi-codegen 2.8.2、openapi-typescript 7.13.0、goimports 0.43.0、golangci-lint 2.15.0。

## 已通过的检查
- 前端：冻结锁文件安装、依赖兼容检查、Prettier、零警告 ESLint、TypeScript、6 项 Vitest 行为测试和生产构建。
- 后端：格式校验、go vet、CGO 单元测试、竞态测试、golangci-lint（零问题）、开发二进制构建。
- 契约：Go 重复生成哈希一致；make check-generated 对前后端生成文件的字节比较一致。
- 生产构建：make build 生成真实前端产物并嵌入 Go 二进制。
- 生产冒烟：make smoke 验证健康响应、请求编号、SPA 深层路径、实际 JS/CSS 资源，以及 API/健康/播放/静态资源缺失时返回 404；临时 SQLite 数据库首次和重复启动、SIGTERM 关闭退出码正常。
- 容器：make docker-build 与 make docker-smoke 通过；UID/GID 10001:10001，验证健康、SPA、静态资源及持久数据重复启动。验证时镜像大小为 50,767,209 字节。
- 真实联调：make test-integration 以临时数据库和随机后端端口启动，通过独立 Vite 代理运行浏览器；桌面与移动端共 4 项通过，CI=1 禁止复用旧服务。
- 文档：规范本地链接有效，git diff --check 通过。

## 环境说明
当前宿主为 NixOS，Playwright 官方 Chromium 因缺少 libglib-2.0.so.0 无法运行。通过 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/etc/profiles/per-user/wenzhengcheng/bin/google-chrome 使用本机 Chrome 完成浏览器检查；已在 README 记录配置。GitHub Actions 配置使用 Ubuntu 与官方下载 Chromium，本地未执行远程 CI。

## 审查记录
独立 Trellis check 已完成：未发现待修复问题。独立重新执行 make check，前后端格式、lint、类型、6 项前端行为测试、后端单元与竞态测试、前端构建和契约重复生成全部通过。最后一次前端暗色主题调整后，主会话重新执行 make build smoke docker-build docker-smoke，全部通过，确认二进制和镜像包含最终前端资源。桌面浅色与移动端暗色截图已检查，布局与状态显示正常。
