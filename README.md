# 115 Media Hub

面向 115 云盘与媒体服务的项目基础骨架。当前实现前后端启动、健康检查、SQLite 迁移和单进程部署；业务功能按后续任务逐步添加。

## 目录

```text
frontend/                 React、Vite、TypeScript 独立项目及 pnpm 锁文件
backend/                  Go、Gin 独立模块及 go.sum
  cmd/mediahub/           服务入口
  internal/               配置、HTTP、SQLite、日志与前端资源
  migrations/             嵌入的 goose SQL 迁移
api/openapi.yaml          前后端共享 HTTP 契约
scripts/                  生产构建、契约一致性与冒烟验证
Makefile                  跨项目编排
Dockerfile                前端构建、CGO 后端编译与 Debian 运行镜像
```

## 环境

固定工具链：Go **1.26.9**、Node.js **24.13.0**、pnpm **11.28.5**。后端 SQLite 驱动依赖 CGO，本机需有 C 编译器（例如 Debian/Ubuntu 的 `build-essential`）。根目录编排还需 Make、Bash、Python 3；容器构建需 Docker。

前端依赖由 `frontend/package.json` 和 `frontend/pnpm-lock.yaml` 固定，后端依赖由 `backend/go.mod` 和 `backend/go.sum` 固定。前后端各自维护项目清单，仓库根目录无需 JavaScript 或 Go workspace。

```bash
make install
```

## 开发

分别在两个终端运行：

```bash
# 终端一，也可执行 make dev-backend
cd backend
go run ./cmd/mediahub
```

```bash
# 终端二，也可执行 make dev-frontend
cd frontend
pnpm dev
```

打开 <http://127.0.0.1:5173>。前端通过同源代理访问后端 `http://127.0.0.1:8080` 的 `/health` 和 `/api`。可通过前端进程的 `MEDIAHUB_BACKEND_URL` 环境变量覆盖代理目标。后端开发启动无需先构建前端。

后端配置见 [backend/.env.example](backend/.env.example)，使用进程环境变量配置；示例文件不会自动加载。例如：

```bash
cd backend
MEDIAHUB_HTTP_ADDR=127.0.0.1:8080 \
MEDIAHUB_DATABASE_PATH=./data/mediahub.db \
go run ./cmd/mediahub
```

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `MEDIAHUB_HTTP_ADDR` | `:8080` | HTTP 监听地址 |
| `MEDIAHUB_DATABASE_PATH` | `./data/mediahub.db` | 本地 SQLite 数据库 |
| `MEDIAHUB_LOG_LEVEL` | `INFO` | 结构化日志等级 |
| `MEDIAHUB_LOG_FILE` | 空 | 可选轮转日志文件，同时保留标准输出 |
| `MEDIAHUB_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭超时 |

SQLite 启动时应用嵌入的版本化迁移，并启用 WAL、外键、busy timeout 和受控连接数。数据库应放在本地磁盘，部署单个应用实例。`GET /health/live` 返回进程存活状态；`GET /health/ready` 检查数据库可访问性，成功响应均为 `{"status":"ok"}`。

## 契约与质量检查

修改 `api/openapi.yaml` 后运行：

```bash
make generate
make check-generated
```

Go 生成器为 `oapi-codegen v2.8.2`，TypeScript 生成器为 `openapi-typescript 7.13.0`。生成文件位于 `backend/internal/http/generated/types.gen.go` 和 `frontend/src/lib/api/generated/schema.ts`，请随契约一起提交，勿手工修改。`check-generated` 对重新生成前后的字节内容进行比较。

```bash
make check                    # 前后端格式、lint、类型、单元测试、构建及契约检查
pnpm --dir frontend exec playwright install chromium
make test-e2e                 # 浏览器行为测试
make test-integration         # 真实后端 + Vite 代理的浏览器联调
```

独立项目检查：

```bash
cd frontend
pnpm format:check
pnpm lint
pnpm typecheck
pnpm test:run
pnpm build
pnpm test:e2e
```

```bash
cd backend
make check
# 包含格式检查、go vet、CGO 单元测试、race 与固定版本 golangci-lint
```

NixOS 等无法运行官方下载 Chromium 的环境，可设置 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指向已安装的兼容 Chrome 可执行文件。标准 CI 使用 Playwright Chromium。

浏览器测试由 Playwright 启动独立 Vite 服务并模拟健康响应，覆盖页面状态与交互。`make smoke` 直接启动真实生产二进制，验证健康检查、SPA 深层路由、保留路径 404、首次及重复数据库启动和 SIGTERM 关闭。`make test-integration` 另行启动临时真实后端，通过 Vite 代理执行浏览器联调。CI 同时执行这些检查与容器验证。

## 生产构建

```bash
make build
make smoke
MEDIAHUB_DATABASE_PATH=/absolute/path/mediahub.db ./bin/mediahub
```

构建先生成真实 `frontend/dist`，再复制到 `backend/internal/web/dist`，最后用 `-tags production` 编译为 `bin/mediahub`；最终二进制嵌入 SPA，发布无需额外静态文件。构建输出均被 Git 忽略。SPA 回退保留 API、健康检查、播放及静态资源错误语义。

## Docker

```bash
make docker-build
make docker-smoke
docker run --detach --name mediahub \
  --publish 8080:8080 \
  --mount source=mediahub-data,target=/data \
  --mount source=mediahub-logs,target=/logs \
  115-media-hub:local
```

打开 <http://127.0.0.1:8080>。镜像以 UID/GID `10001:10001` 运行，数据库在 `/data/mediahub.db`，轮转日志在 `/logs/mediahub.log`。绑定宿主目录时需给该 UID/GID 写权限；命名卷可直接使用。持久保留数据卷，升级前使用一致性 SQLite 备份。镜像采用 Node/Go 构建阶段和兼容的 Debian Bookworm 运行阶段，支持 SQLite CGO。
