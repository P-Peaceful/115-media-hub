# 实施计划

## 前置边界

行为缺口是服务端尚无认证/会话/CSRF，前端尚无登录和认证路由保护。改动预期覆盖配置、认证服务、HTTP/OpenAPI、React 路由/页面、部署示例和开发文档；不新增数据库表，不实现播放认证、用户体系、改密页面或 refresh token。开始实施前需用户明确批准本规划摘要。

## 有序步骤

1. **配置和启动失败关闭**
   - 在 Config 加入管理员用户名/密码字段及 ENV 覆盖。
   - 增加显式 `cookie_secure` / `MEDIA_HUB_COOKIE_SECURE` 布尔设置，默认 false，并在 HTTPS 部署说明中要求启用。
   - 校验凭据存在且非全空白；扩展 YAML 示例、Compose 注入、`.env.example` 和运维说明。
   - 编写配置用例覆盖 YAML、ENV、字段级优先级、空 ENV 和非法配置。
2. **内存认证服务**
   - 建立登录验证、随机会话/CSRF 凭据、24 小时固定过期、查询/撤销能力。
   - 确保过期会话不可用、重启自然失效，秘密不进入日志；编写服务单测。
3. **HTTP 契约和路由保护**
   - 更新 OpenAPI，定义 auth DTO、状态码和 Cookie/CSRF 行为，运行 `make generate`。
   - 实现登录、当前会话、退出 handler；增加认证及 CSRF 中间件。
   - 保持健康探针公开；为管理路由提供明确保护组，并用路由测试验证拒绝/允许行为。
4. **前端登录体验**
   - 按 Canva UI 设计实现“欢迎回来”登录界面、管理员账号/密码表单、部署凭据说明及 115 授权分离提示，包含提交状态和统一失败反馈。
   - 增加路由 guard；登录后恢复经过校验的站内目标；会话失效保留非敏感筛选上下文。
   - CSRF token 只保存在内存，由请求 helper 注入变更请求；增加前端组件/路由/API 用例。
5. **文档与安全审阅**
   - 检查 ENV/YAML/Compose 配置示例及修改凭据后重启说明一致。
   - 检查日志、Cookie 属性、跳转校验、存储 API 使用和 API 路由覆盖。
6. **全量检查**
   - `make generate`（如 OpenAPI 有变化）、`make check`、`make smoke`、`pnpm --dir frontend build`。
   - 手工核对数据库仍无身份/会话表，健康探针可用，嵌入式 SPA 深链正常。
   - 最终检查所有变更文件和文档引用，再进入 Trellis 质量审查。

## 高风险点与回滚点

- 配置必填会使未注入凭据的部署启动失败；先更新 Compose 和部署说明，确保失败消息不泄漏值。
- Gin catch-all SPA fallback 可能使未注册 API 落入静态处理；认证路由应先于 fallback 注册，未知 `/api` 路径仍返回 404。
- 24 小时 TTL 必须由服务端强制，不能仅依赖 Cookie Max-Age。
- API 和 SPA 契约跨后端/前端生成代码，改 OpenAPI 后必须检查两个生成产物。
- Cookie Secure 与 TLS 终止配置不能从不可信代理头推导；仅接受显式部署信任边界。
- 无迁移。若 auth API/路由设计返工，可独立回滚 API 与 auth 包；部署文档和配置字段需同步回滚。

## 进入实施前确认

- [x] 用户审阅并明确批准最新规划摘要。
- [x] 子代理上下文清单 `implement.jsonl`、`check.jsonl` 含真实 spec 条目，并通过 `task.py validate`。
- [x] 按 `trellis-before-dev` 重新确认改动边界、spec 检查清单和实际文件。
