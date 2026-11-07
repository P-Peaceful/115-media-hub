# 前端实现与验证记录

## 实现内容与职责归属

前端实现已完成；主代理负责最终集成评审和任务关闭。`features/auth/api.ts` 负责一次性校验生成 DTO；`lib/api/client.ts` 负责安全错误、中止传播、同源 fetch 和受保护请求 401 通知。`auth-provider.tsx` 负责 bootstrap、仅驻留内存的 CSRF/会话状态迁移、固定过期定时器、焦点/重连刷新及请求取消/缓存清理。路由边界负责初始化/登录/受保护导航。表单组件负责草稿、客户端校验、提交中和安全错误展示。`AuthenticatedHome` 将登出操作注入现有且独立测试的健康页面。

未手动修改前端依赖或生成契约文件。密码、CSRF、会话令牌或认证布尔值均不写入浏览器存储。登录返回地址会拒绝外部/协议相对、控制字符/反斜杠和认证页目标，包括编码形式。当前初始化默认输出目录为 `/app/media`，服务 URL 为浏览器源；有效期选项为 1/7/30 天，默认 7 天。校验与服务端一致，覆盖 8–32 个可打印非空格 ASCII 密码、用户名字符、仅允许源的 URL 规则、规范化绝对服务器路径，以及格式/控制字符/长度边界。

## 组件/API 评审

实现前查阅了 HeroUI MCP 组件目录和锁定版本 v3.2.7 的所选组件完整文档。标准控件使用 HeroUI Form、TextField、Label、FieldError、Description、InputGroup（含 prefix/input/suffix）、Button、Checkbox（Control/Indicator/Content）、Select/ListBox、Card、Modal、Link、Alert 和 Spinner。装饰图标由 Lucide 提供。语义化 main/section/aside/标题和 CSS 负责文档布局/说明文字，不替代库的交互行为。媒体文件夹插画是本地 SVG，包含分层文件夹面、紫色渐变、玻璃边缘、云、播放符号和阴影/反射。

核对过的官方当前文档：

- 通过 `list_components` 后，对实际采用的 HeroUI v3.2.7 控件调用 `get_component_docs`，包括复合 Select、InputGroup、Checkbox 和 Modal API。
- React 19.3：https://react.dev/reference/react-dom/components/form （当前 `onSubmit` 事件处理）；并核查安装的 React 类型标注。新处理器导入受支持的 `SyntheticEvent`，避开第三方回调类型中仍存在但已弃用的 `FormEvent` 别名。
- TanStack Query v5：https://tanstack.com/query/latest/docs/reference/QueryClient （`cancelQueries`/`removeQueries`/`setQueryData` 和缓存归属），与安装的 5.106.0 一致。
- React Router 8.5.1：https://reactrouter.com/api/declarative-routers/BrowserRouter （BrowserRouter、Route/Routes、Navigate/Outlet、location/search hooks）。
- 已检查 HeroUI 和 Lucide 安装声明中的弃用标记；未使用弃用属性/类型。未引入 HeroUI v2 写法、不稳定 API 或无必要依赖升级。

Vite 代理必须显式设置 `changeOrigin: false`。字符串简写会把 Host 改写为后端目标端口，而浏览器 Origin 仍是前端端口，后端同源 CSRF 校验因此正确拒绝请求。`/api` 和 `/health` 均已保留 Host；真实集成验证该边界，不依赖转发头。

## 命令与结果

- `pnpm --dir frontend test:run`：**76/76 通过**，共 6 个文件。覆盖密码边界/Unicode/空格/控制字符（含末尾 LF/CR）、保留原始 33 位粘贴且拒绝提交、无效/不一致 DTO、中止请求、安全错误、CSRF 提交、错误格式的受保护 401、字段错误、网络重试、加载、密码显示、恢复对话框焦点、初始化确认/默认值、本地返回地址校验、过期、焦点恢复时撤权、以及迟到的受保护结果取消/缓存删除。现有健康检查测试继续通过。
- `pnpm --dir frontend format:check`：通过。
- `pnpm --dir frontend lint`：通过，0 警告。
- `pnpm --dir frontend typecheck`：通过，严格/exact optional 选项保持启用。
- `pnpm --dir frontend build`：通过。Vite 提示主 JavaScript 包约 592 KB，属于非阻断提示；未任意屏蔽阈值或推测性拆分代码块。
- 默认下载的 Playwright 浏览器在此 Nix 主机因缺少 `libglib-2.0.so.0` 无法运行。改用已安装的兼容浏览器执行 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/etc/profiles/per-user/wenzhengcheng/bin/google-chrome pnpm --dir frontend test:e2e`：**6 项通过**，模拟模式有 2 项仅真实后端公开健康检查按设计跳过。
- 代理修复后，主代理运行隔离的真实后端集成：**桌面端 4/4、暗色移动端 4/4** 通过。每个项目的单一有状态测试按序执行初始化 → 刷新 → 登出 → 恢复说明 → 错误凭据 → 关闭记住登录的有效键盘登录 → 登出，并检查实际 Cookie 持久化标志。其他认证/健康浏览器场景模拟认证或只读取公开健康检查。

## 视觉验证

主代理已将 `screenshots/` 中的页面与移除完整顶部状态栏和 115 授权卡后的参考画板对照并检查。桌面比例/内容密度、紫色渐变、圆角卡片阴影和文件夹风格均得到保留。首轮截图发现复选框堆叠、选择器行对齐和窄屏插画/文字重叠，已修正并重新截图。桌面/移动端亮暗主题均无水平溢出，移动端主视觉已在网格中预留空间。最终截图和集成门禁证据由主代理维护。

## 尚存限制

- 主 JS 包体积提示见上文；没有功能门禁失败。
- `/app/media` 是经过校验的服务器路径设置；表单不浏览服务器文件，也不保证文件系统权限。
- 本地 SVG 重绘了参考插画的视觉方向，并非对原位图的逐像素提取。
- 完整应用/构建/容器结果和最终整体验收由主代理记录。

## 密码策略后续

用户最新要求将密码限制为 8–32 个可打印非空格 ASCII 字符（U+0021–U+007E），不强制字符组合。登录和初始化页显示一致简短提示，确认密码复用同一校验器。校验器显式检查长度并拒绝范围外字符，不使用可能放过末尾换行的 JavaScript `$` 结束锚点。

按主代理澄清，不使用原生 `maxLength`、裁剪、手动切片或自定义粘贴框架。保留超长原始输入，由 HeroUI 校验拒绝提交。组件测试粘贴 33 个原始字符，验证完整值仍在、字段错误可见且提交次数为 0；其他测试拒绝中文/空格，接受 32 个重复 ASCII 数字。现有浏览器工作流也会先输入 33 位初始化密码，检查原值未变并出现两个校验错误，再进行有效初始化。

最新后续检查：聚焦测试 59 项通过；前端全量 76/76 测试、格式、Lint、严格类型检查和构建通过。模拟桌面/移动 E2E 通过 6 项，2 项仅真实后端检查按设计跳过。此前真实后端集成早于本次密码策略更新；主代理负责最终策略的隔离真实集成和生产组装复验。此验证期间未修改用户数据库、账户或进程。源码记录完成后保持稳定。
