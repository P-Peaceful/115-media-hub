# SDK 与抽样预览验证记录

状态：SDK 代码及独立审查完成，相关浏览器和生产/容器冒烟通过；最终全应用集成随父任务在附属同步落地后执行，尚未归档。基线 61ecef8。

## 已完成

- 新 adapter 的 synctest/race 覆盖：完成后间隔、调大间隔、前后台公平、取消/Close、共享冷却、Retry-After 秒/日期/不可表示长值、最多三次尝试、发送预算、端点/响应边界和脱敏。
- Inspect 在释放准入前识别 HTTP 200 业务限流；旧凭据响应被废弃但同账号冷却保留。网络正文/关闭 I/O 故障归 Temporary；已知 HTTP auth/notfound 先于坏正文分类。
- 后台调用总预算为 max(45s, interval+15s)，使60秒合法配置可读取连续页；caller更短期限仍优先。Details可选容量的adapter自身预算超时保留有效profile，caller取消/凭据变化仍失败。
- Service 账号、QR、Files、播放接入；保留公开 200、播放器空 UA、m115 和可信目标。最后凭据复核窗口保留 ErrCredentialChanged/ctx/cause，扫描立即停止而不继续 siblings。
- 快速快照正常成功及快慢同树候选/排除等价；1151 文件与 5001 目录两种分页共 7 请求；计数变化、重复、提前结束、错路径、断链/环、根变化、业务 auth/throttle、不支持/关闭/根0等覆盖。完整路径深度最多256（包括缓存前缀）。标准目录最多10000 page-call轮次，每轮最多3 HTTP尝试；超限不采用该目录。
- 预览 3 目录/5 次实际请求含重试/15s/20样例、页面采用、根未验证错误、取消、零写入及空样本确认；前端独立静态 query key 防宽泛失效、焦点与重连重扫，显式刷新和离开取消。
- `make -C backend check`：首次全套通过（format/vet/test/race/lint）。后续独立审查修复后，adapter/provider115/strm/app/http 的 targeted race、全 format-check/vet 通过；最终 lint 见审查记录。
- `make check-generated`：Go/TS 重复生成一致。
- 前端 format/lint/typecheck 已通过。完整 Vitest 15文件/146测试串行通过：`pnpm --dir frontend test:run --maxWorkers=1 --testTimeout=15000`。默认并发/5秒单场预算在此Nix环境有超时；适配命令通过，不将默认 make check-frontend 冒称成功。

## 环境与既有测试漂移

- 固定 Node 24.13.0：`/nix/store/sy0c7j0npsq33d9zhnnzvjnzc52f4y0p-nodejs-24.13.0/bin`。
- Chromium 缺动态库，通过 `LD_LIBRARY_PATH=/nix/store/zwgl9ci65rjyxndijj1fw9cnn373bp50-listen1-2.33.0-fhsenv-rootfs/usr/lib64` 补齐，不改产品依赖。
- HEAD 详情页本就未渲染暂停/启用按钮，仅遗留未使用绑定；移除无用 import/destructuring 并修正这部分旧单测/e2e断言，保留状态显示与列表页启停覆盖。没有为测试恢复已移除功能。
- OpenAPI 补 `sample_limit`，准确表示20样例边界；用户批准的四项预算不变。

## 前端最终验收与浏览器实际记录

- `pnpm --dir frontend format:check`、`lint`、`typecheck`、`build` 均通过；生产构建 3.80 秒，保留现有大于 500 kB 的 chunk 提示，未扩展为分包重构。
- 浏览器使用上述 Node/FHS 动态库环境，`--workers=1 --timeout=90000`。桌面和 mobile-dark 共 18 个不同场景均取得最终通过证据，包括抽样预算语义、输出受阻、授权失败、版本刷新、空样本仍可确认、任务提交/跳转、清单操作、分页及窄屏导航。
- 如实保留运行过程：两文件首轮为 **16 passed / 2 failed**（5.4 分钟）；失败为 mobile 文档重载和清单初载尚未收到接口响应时，原 5 秒 UI 断言已开始。只修改测试就绪边界，先等待对应配置/清单响应，保留原 UI、只读、确认与导航断言，不加 sleep，不修改产品行为。
- 定向复查 10 场为 **9 passed / 1 failed**（2.9 分钟）：原两处失败均通过，另一个 desktop 直接文档导航暴露同类就绪问题。统一媒体库文档导航/重载等待配置响应后，最终单跑 `media-library-detail.spec.ts` 为 **8 passed**（2.2 分钟）。`strm-management.spec.ts` 的其余 8 场沿用首轮通过证据，改过的清单交互场景在定向复查中 desktop/mobile 两场均通过；没有将以上记录冒称单次 18 场全绿。
- 最后测试辅助函数调整后，针对两个 e2e 文件的 ESLint、完整 TypeScript 检查与格式检查通过。应用代码未再改动，因此未重复已通过的 146 个单元测试和生产构建。
- 运行日志：`/tmp/sdk-preview-vitest-final.log`、`/tmp/sdk-preview-build-final.log`、`/tmp/sdk-preview-e2e-final.log`、`/tmp/sdk-preview-e2e-recheck.log`、`/tmp/sdk-preview-e2e-media-final.log`。

## 待完成

独立全scope审查已通过，详见 sdk-review.md。`make build smoke` 在最终SDK源码上再次通过；`make docker-build docker-smoke` 通过（该Docker快照早于后台长间隔预算的最后修正，父任务将对最终源码重建/冒烟）。完整两项目真实后端集成在附属同步后对最终工作区统一运行，未通过前不归档本任务。

真实 115 网盘未实测。所有上游行为证据为固定源码参考和模拟协议；不承诺私有端点稳定或免限流。
