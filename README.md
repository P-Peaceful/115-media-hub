# 115-media-hub

围绕 115 网盘媒体文件的 STRM 与元数据管理项目。目前已完成前后端基础架构和单管理员登录，媒体业务功能尚未实现。

## 工程结构

- `backend/`：Go + Gin + GORM + SQLite + goose。
- `frontend/`：React 19 + Vite + TypeScript + HeroUI v3 + Tailwind CSS v4。
- `api/`：共享 OpenAPI 契约与生成配置。
- `deploy/`：Docker 与 Compose。
- `scripts/`：前端资源复制及联合冒烟验证。
- `docs/`：架构、开发和初始化记录。

## 本地开发

需要 Go 1.26+、C 编译器、Node.js 22.12+、pnpm 11.28.5、Make。具体依赖版本见 `backend/go.mod` 与 `frontend/package.json`。

```sh
make install
```

先配置管理员用户名和密码。在两个终端分别运行：

```sh
export MEDIA_HUB_ADMIN_USERNAME=your-admin-name
export MEDIA_HUB_ADMIN_PASSWORD=replace-with-a-long-random-password
make dev-backend
```

```sh
make dev-frontend
```

访问 http://127.0.0.1:5173 。Vite 将 `/api`、`/p`、`/health` 代理到 http://127.0.0.1:8080 。后端单独运行时不提供前端资源，首页返回 503 并提示使用 Vite。

## 检查与构建

```sh
make generate       # 从同一契约生成 Go 接口与前端类型
make check          # Go 格式/vet/测试，前端类型/lint/peer 检查/测试
make build          # 前端打包 → 复制资源 → CGO Go 嵌入构建
make smoke          # 联合构建并运行真实服务冒烟测试（需要 Python 3）
```

构建结果：`backend/bin/115-media-hub`。从 `backend/` 目录启动：

```sh
cd backend
MEDIA_HUB_ADMIN_USERNAME=your-admin-name \
MEDIA_HUB_ADMIN_PASSWORD=replace-with-a-long-random-password \
./bin/115-media-hub
```

`GET /health/live` 检查进程，`GET /health/ready` 检查数据库。管理端使用配置凭据登录，服务端会话自创建起固定 24 小时过期；没有内置默认凭据。也可以用 `-config` 指定包含 `admin_username` 和 `admin_password` 的 YAML 文件，详见[开发说明](docs/development.md)。媒体业务 API 暂未实现。

## Docker

先复制 `.env.example` 为 `.env`，填写 `MEDIA_HUB_ADMIN_USERNAME` 和 `MEDIA_HUB_ADMIN_PASSWORD`，再运行：

```sh
docker compose -f deploy/compose.yaml up --build -d
```

访问 http://127.0.0.1:8080 。容器使用非 root 用户和 Debian 运行环境，构建启用 CGO。数据、STRM 输出、隔离区、缓存、日志、备份分别使用命名卷。后续接入 Emby/Jellyfin 时需按真实环境配置共享卷和路径映射。

架构与边界见 [架构说明](docs/architecture.md)，配置与工作流见 [开发说明](docs/development.md)。
