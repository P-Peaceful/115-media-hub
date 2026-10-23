# 初始化验证记录

日期：2026-10-23（Asia/Shanghai）。范围：前后端基础架构，无业务实现。

| 验证 | 结果 |
| --- | --- |
| `make check` | 通过：Go 格式、vet、Go 测试、TS 类型、ESLint、peer 检查、Vitest |
| `make generate` | 通过：再次生成的 Go 和 TypeScript 文件 SHA-256 与原文件一致 |
| `make smoke` | 通过：真实 CGO 二进制、SQLite 迁移、健康接口、嵌入首页、SPA 深链、404、无业务表、SIGTERM 退出 |
| `docker compose -f deploy/compose.yaml config --quiet` | 通过 |
| `docker build -f deploy/Dockerfile -t 115-media-hub:local .` | 通过：前端构建、Go CGO 构建、Debian 运行镜像 |
| 临时 Docker 容器 | 通过：非 root 用户、数据库 readiness、嵌入页面、API 404、镜像健康检查命令 |
| 浏览器查看嵌入前端 | 页面渲染正常，控制台无 error/warn |
| 独立只读代码审查 | 无 P1/P2 问题 |

Go 测试覆盖配置优先级及非法配置拒绝、SQLite WAL/外键/锁等待/单连接/重复迁移、健康状态、SPA 与保留路径的行为。Vitest 的 5 项测试覆盖请求数据、204、错误状态、CSRF 请求头及跨源 URL 拒绝。

测试容器与临时服务已停止。镜像 `115-media-hub:local` 和本地构建产物保留供使用。没有执行部署、Git 提交或远端推送。115、Emby/Jellyfin、登录、任务与回传业务均未接入或验证。

## 项目更名复验

项目标识已统一为 `115-media-hub`，环境变量前缀统一为 `MEDIA_HUB_`，STRM 文件格式及相关业务目录名保留。

- `make generate`、`make check`、`make smoke` 与 Compose 配置检查通过。
- 新名称镜像 `115-media-hub:local` 构建通过；临时容器验证了新入口二进制、`mediahub` 用户、页面标题、默认数据库文件及 readiness。
- 已清理旧名称本地二进制；未迁移或删除任何已有运行数据库、Docker 卷或镜像。
- 本地构建跳过 VCS 信息嵌入，以适配无有效 Git 元数据的工作目录。
