# 初始化技术设计

## 边界和目录
采用同级的 frontend/ 与 backend/。frontend/ 管理 package.json、pnpm-lock.yaml、Vite/TypeScript/测试配置和 src/。backend/ 管理 go.mod、go.sum、cmd/mediahub/、internal/、migrations/ 和后端工具配置。api/openapi.yaml 保留在仓库根目录，作为共享传输契约来源；根目录编排脚本、Makefile 和容器配置只负责跨项目操作。不额外创建根 JavaScript workspace 或 Go workspace。

frontend/src/ 内保留现有 app、features 和共享模块的依赖方向，统一配置 @/ 别名。backend/ 内保留构造器注入，以及 internal/app、internal/http、internal/repository/sqlite 的职责边界。只创建骨架实际使用的包。

## 运行时和传输契约
初始前端配置 Router 和 Query providers，使用文档确认的 HeroUI v3 组件展示健康状态。开发时通过 Vite 同源代理发起健康检查。健康端点保持最小化，无需身份认证。OpenAPI 定义相关 DTO 和错误响应；通过固定版本的工具生成 TypeScript 和 Go 传输类型。已实现的 HTTP 错误遵循现有请求 ID 与错误信封约定。

后端启动时读取文档约定的环境变量配置，以 WAL、外键、busy timeout 和受控连接数及写并发打开本地 SQLite，执行嵌入的 goose 迁移，再启动 Gin。就绪检查验证数据库访问，存活检查验证进程可提供服务。使用 slog 和支持信号驱动优雅关闭的 HTTP 服务。尚无使用方的业务凭据配置暂不引入。

## 生产构建
先构建 frontend/dist，将真实输出复制到 backend/internal/web/dist，再编译嵌入资源的 Go 二进制。提供不依赖伪造前端产物的开发构建路径，例如使用构建标签区分资源装配。忽略复制后的构建资源和二进制产物。多阶段 Docker 构建提供 CGO 和 C 编译器支持，并使用兼容的 Debian 运行环境。SPA 回退只适用于前端路由；API、健康检查、播放和静态资源路径必须保留独立处理逻辑。

## 取舍和兼容
本次初始化 SQLite 与迁移，以验证选定的后端基础架构；业务模型和服务后续实现。生成的传输契约与持久化模型分离。采用用户要求的独立根目录，同步修订现行规范中旧的 web/ 路径和仓库根目录 Go 命令。

## 回退
本次为全新骨架，无现存应用数据。回退时可移除本次新增应用和构建文件，保留已有开发工具。开发数据库放在忽略路径，测试使用临时数据库。不引入破坏性迁移。
