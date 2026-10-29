# 项目内 115 客户端替换

## Goal
参考 pancras-loe/115-station 的 Cookie 接口实现，移除 SheltonZhu/115driver，解决目录 ID 类型变化导致路径反序列化失败，同时保留已上线行为。

## Confirmed facts and authorization
- 用户已选择项目内客户端，并认可上轮完整设计；2026-10-30 明确要求先创建任务再实施，主要参考 115-station。
- 当前适配层 SDK 使用集中在 backend/internal/adapter/drive115；服务、HTTP、凭据 JSON 都有项目自有边界。
- 故障证据：Stat 的 paths[].file_id 上游为字符串，SDK 为 int，触发 json.UnmarshalTypeError。

## Requirements
R1. 完全移除运行代码、测试、go.mod/go.sum 中的 115driver 依赖；在现有适配层实现所需协议。
R2. 保留 web/alipaymini/wechatmini 扫码、状态轮询、完成登录、身份与根目录验证、账号及容量信息。
R3. 保留目录单页列表、搜索、类型筛选、面包屑；路径参考 /files 的 path 祖先链，ID 无损兼容字符串和数字。
R4. 保留登录/浏览 API、数据库模式与凭据 JSON；按用户新增要求，将账号资料的数字会员等级改为上游会员名称，规范与生成的前后端契约同步更新。
R5. 校验 HTTP 状态、业务状态、目录身份；分类登录失效、限流、网络异常；不泄露凭据、签名、上游响应或请求 URL。
R6. 请求可取消、有超时，复用连接池；不共享可变 Cookie；重复/并发请求不串凭据。
R7. 会员显示采用 Cookie vip_limit 的 data.user_limit.level_name 原文；无文本/请求失败时显示未知，移除数字会员等级和数字推导名称。旧快照无名称时同样未知；昵称、头像、容量仍只采用上游值，不透传未审核的原始响应。

## Acceptance Criteria
- [x] 数字、字符串及超出 JS 安全整数范围的 ID 均无损生成正确面包屑，重复调用正常。
- [x] 已删除/错误目录返回错误，不伪装为根目录或空成功。
- [x] 三种扫码设备、全部扫码状态及换取原格式凭据的行为由模拟上游测试证明。
- [x] 验证/资料查询保留已知账号证据，区分必需验证和可选容量丰富；既有 service/HTTP 测试通过。
- [x] HTTP/业务错误、异常 JSON、取消、并发隔离及脱敏有回归测试。
- [x] go list -m all 不含 115driver；make check 通过，适配层 race 通过。

- [x] 新会员名称经适配层、快照、API、连接页与侧栏保留原文；无名称、旧快照、查询失败和未知字段不会生成数字等级/自造名称，也不泄露凭据。

## Out of Scope
上传、下载、离线任务、OpenAPI Bearer 授权、多账号业务、数据库路径缓存、无关 UI/API 改动、自动提交。真实账号验证须单独说明，不能用模拟测试代替。
