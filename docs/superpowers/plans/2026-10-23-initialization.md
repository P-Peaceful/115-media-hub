# 项目基础架构初始化 Implementation Plan

> 执行方式：当前会话直接实施；用户已确认目录并授权初始化，不再追加设计审批。使用 executing-plans 持续执行。

**Goal:** 建立可以独立开发、联合构建的前后端工程，不编写业务代码。
**Architecture:** backend 独立 Go module，frontend 独立 pnpm 工程，共享 OpenAPI。生产嵌入前端资源，开发通过代理连接。
**Tech Stack:** Go、Gin、GORM、SQLite、goose、React 19、Vite、TypeScript、HeroUI v3、Tailwind v4。
**Spec:** docs/architecture.md

## Global Constraints
- 只允许基础架构实现；业务模块仅预留目录。
- SQLite 使用 CGO；依赖由 go.sum 与 pnpm-lock.yaml 锁定。
- 保留已有 .git/.agents 等环境目录，不初始化或改写 Git 元数据。

## Review Focus
- 数据库不可用时 readiness 失败，liveness 仍成功。
- SPA 回退不能吞掉 API 或缺失静态资源的 404。
- 非法启动配置应阻止启动。
- 前端产物缺失时 Go 开发构建仍可编译，生产通过构建脚本保证资源复制。
- 日志文件权限与数据库连接生命周期必须正确处理。

### Task 1: 后端基础
- [x] 添加配置、数据库、健康检查和静态资源托管的行为测试，运行并观察失败。
- [x] 实现配置加载与校验、slog 轮转、SQLite/goose、Gin 路由与优雅关闭。
- [x] 运行 Go 测试、vet 和 CGO 构建。

### Task 2: 前端基础
- [x] 建立 Vite/TS/HeroUI/Tailwind、Router、Query Provider、主题 Store 和请求基础设施。
- [x] 添加请求错误处理测试，验证类型检查、lint、测试及构建。
- [x] 生成前后端健康检查契约代码，验证生成命令可重复执行。

### Task 3: 联合交付
- [x] 添加 Makefile、复制脚本、多阶段 Dockerfile、Compose 和开发文档。
- [x] 联合构建，启动真实进程检查健康接口、SPA、404 和信号退出。
- [x] 记录可用检查结果及环境限制，核对无业务实现。

## 执行记录

- 按用户明确授权直接执行已确认的目录和初始化范围；未追加审批。
- 环境中的 `.git` 目录无有效 Git 元数据且只读，直接在指定工作区创建文件，未创建 worktree 或执行提交。
- TypeScript 最新版 7 与 OpenAPI/ESLint 工具的 peer 约束不兼容，改为精确锁定兼容的 5.9.3；依赖 peer 检查通过。
- SPA 深链测试发现 Gin NoRoute 预设 404，修正 HTML 回退的显式 200；回归测试通过。
- 环境无 `/bin/bash`，Makefile 改用 PATH 中的 POSIX sh。
- 验证：make check、make smoke、make generate、Compose 配置检查、实际 Docker 构建与临时容器验证通过。
- 只读审查未发现 P1/P2 问题；业务目录仅占位，无业务表或业务成功接口。
