# 115 Media Hub

面向 115 云盘与媒体服务的媒体管理系统。当前提供管理员首次初始化、登录与会话、服务器本地密码重置，以及健康检查、SQLite 迁移和单进程部署；媒体业务功能按后续任务逐步添加。

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
MEDIAHUB_LOG_FILE=./data/logs/mediahub.log \
go run ./cmd/mediahub
```

本地开发命令显式把数据库和日志放在项目目录下；后端默认数据库为 `./data/mediahub.db`，文件日志默认关闭。Docker 镜像会单独设置容器内路径，统一位于 `/app/` 下。

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `MEDIAHUB_HTTP_ADDR` | `:8080` | HTTP 监听地址 |
| `MEDIAHUB_DATABASE_PATH` | `./data/mediahub.db` | 项目目录内的 SQLite 数据库；容器内由镜像覆盖为 `/app/data/mediahub.db` |
| `MEDIAHUB_LOG_LEVEL` | `INFO` | 结构化日志等级 |
| `MEDIAHUB_LOG_FILE` | 空 | 可选轮转日志文件，同时保留标准输出；容器内由镜像设置为 `/app/logs/mediahub.log` |
| `MEDIAHUB_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭超时 |
| `MEDIAHUB_COOKIE_SECURE` | `false` | 外部 HTTPS 部署使用安全 Cookie，反向代理终止 TLS 时设为 `true` |

SQLite 启动时应用嵌入的版本化迁移，并启用 WAL、外键、busy timeout 和受控连接数。数据库应放在本地磁盘，部署单个应用实例。`GET /health/live` 返回进程存活状态；`GET /health/ready` 检查数据库可访问性，成功响应均为 `{"status":"ok"}`。

## 管理员初始化与登录

首次打开应用会进入初始化页。创建唯一管理员并配置服务访问地址、默认输出目录和登录有效期后，自动登录进入当前首页。后续访问需要登录；登录与初始化页均不显示顶部状态栏或 115 授权入口。115 接入由后续连接管理模块提供。

- 用户名为 3–32 个 ASCII 字母、数字、下划线、句点或连字符，区分大小写；密码为 8–32 位英文字母、数字或英文符号，不支持中文、空格和控制字符，也不强制混合字符类型。
- 服务访问地址预填当前浏览器地址，接受 HTTP(S) 源地址；当前部署在根路径，不支持地址中的路径前缀、用户名密码、查询或片段。
- 默认输出目录为 **`/app/media`**，表示服务器或容器内的绝对路径。初始化保存路径配置，尚不生成媒体文件，也不代表已验证该路径可写。
- 登录有效期可选 1、7、30 天，默认 7 天。勾选“记住登录状态”使用持久 Cookie，否则使用浏览器会话 Cookie；服务端固定到期时间始终有效。浏览器恢复上次会话的行为由浏览器决定。

会话通过 HttpOnly、SameSite=Lax Cookie 传递，写请求校验 CSRF。前端不将密码、会话或 CSRF 令牌写入 localStorage/sessionStorage。若 HTTPS 在反向代理处终止，请为后端配置 `MEDIAHUB_COOKIE_SECURE=true`，通过同源代理访问；直接使用 HTTP 的本地开发环境保持默认 `false`。

### 管理员密码恢复

登录页的“忘记密码”说明服务器本地恢复方式，不提供公开的密码重置接口或邮件找回。先停止应用，然后针对**同一个数据库**运行维护命令，按提示隐藏输入新密码及确认密码：

```bash
MEDIAHUB_DATABASE_PATH=/absolute/path/mediahub.db ./bin/mediahub admin reset-password
```

成功后重新启动应用。此操作只重置现有管理员的密码并撤销全部旧会话，保留系统配置；未初始化数据库不能通过该命令创建管理员。不要删除数据库来恢复密码。自动化输入可显式使用 `--password-stdin`，不要把密码放在命令行参数或环境变量中。

Docker 命名卷部署可使用以下命令维护，然后启动原容器：

```bash
docker stop mediahub
docker run --rm --interactive --tty \
  --mount source=mediahub-data,target=/app/data \
  --mount source=mediahub-logs,target=/app/logs \
  115-media-hub:local admin reset-password
docker start mediahub
```

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

浏览器测试由 Playwright 启动独立 Vite 服务，使用模拟 API 覆盖认证和页面状态与交互。`make smoke` 直接启动真实生产二进制，验证健康检查、SPA 深层路由、保留路径 404、真实初始化、跨进程重启的会话持久化、实际密码恢复命令与旧会话/密码失效、首次及重复数据库启动和 SIGTERM 关闭。`make test-integration` 为桌面和移动端浏览器项目分别启动使用独立临时数据库的真实后端，通过 Vite 代理执行完整初始化、退出和重新登录联调，避免多个项目竞争同一首次初始化。CI 同时执行这些检查与容器验证。

## 生产构建

```bash
make build
make smoke
MEDIAHUB_DATABASE_PATH=/absolute/path/mediahub.db ./bin/mediahub
```

构建时可通过 `MEDIAHUB_VERSION=v1.2.3 make build` 注入版本；Docker 构建可使用 `--build-arg MEDIAHUB_VERSION=v1.2.3`。未指定时版本显示 `dev`，不会推断是否为最新版本。总览的运行时长和版本由需要管理员会话的 `GET /api/v1/system/status` 提供。

构建先生成真实 `frontend/dist`，再复制到 `backend/internal/web/dist`，最后用 `-tags production` 编译为 `bin/mediahub`；最终二进制嵌入 SPA，发布无需额外静态文件。构建输出均被 Git 忽略。SPA 回退保留 API、健康检查、播放及静态资源错误语义。

## Docker

```bash
make docker-build
make docker-smoke
docker run --detach --name mediahub \
  --publish 8080:8080 \
  --mount source=mediahub-data,target=/app/data \
  --mount source=mediahub-logs,target=/app/logs \
  --mount source=mediahub-media,target=/app/media \
  --mount source=mediahub-cache,target=/app/cache \
  --mount source=mediahub-quarantine,target=/app/storage/quarantine \
  --mount source=mediahub-backups,target=/app/backups \
  115-media-hub:local
```

打开 <http://127.0.0.1:8080>。镜像以 UID/GID `10001:10001` 运行，数据库在 `/app/data/mediahub.db`，轮转日志在 `/app/logs/mediahub.log`，默认媒体输出目录为 `/app/media`。应用数据目录统一位于 `/app` 下：可重建缓存使用 `/app/cache`，备份使用 `/app/backups`，隔离区使用 `/app/storage/quarantine`。绑定宿主目录时需给该 UID/GID 写权限；命名卷可直接使用。持久保留数据卷，升级前使用一致性 SQLite 备份。镜像采用 Node/Go 构建阶段和兼容的 Debian Bookworm 运行阶段，支持 SQLite CGO。

### 设置与 115 账号

登录后从侧栏进入“设置”，可修改管理员用户名、密码和后续登录的有效期。修改用户名或密码需验证当前密码；成功后其他管理员会话失效，当前会话保留。登录页的“记住登录状态”仍决定 Cookie 是否跨浏览器会话保留。

“115 账号”支持使用 115 App 扫码登录一个账号，重新扫码成功会替换原有授权。二维码过期、取消或上游不可用时可重新获取。账号页面、总览和侧栏读取 115 返回的账号及容量信息；容量暂不可用时不会显示为零。115 的接口可能发生变化，实际可用性取决于上游服务及网络环境。

根据个人内网使用的选择，115 凭据以明文保存在应用数据库，不需要额外主密钥；数据库及其备份均包含凭据，请按自己的部署方式管理文件访问权限。
