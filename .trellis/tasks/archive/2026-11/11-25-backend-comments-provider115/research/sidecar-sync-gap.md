# 媒体库附属文件同步缺口

日期：2026-11-25；用户在尚未开始实现的规划阶段补充：STRM 生成时未按媒体库配置下载 NFO、海报封面等附属文件。

## 已验证事实

- `backend/internal/service/library/service.go:27`、`:28` 定义 GenerateNFO、GeneratePosterCover；前端编辑/详情可保存和显示这两项设置。
- `backend/internal/service/strm/scan.go:147` 仅将 FileType=video 的文件加入源清单；图片及 .nfo 等其他文件被忽略。
- `backend/internal/service/strm/generate.go:126` 将源清单逐项交给 generateFile；`write.go:16` 仅处理 STRM URL 内容及产物所有权，不下载附属文件。
- `backend/internal/service/strm/types.go:38` 的 Provider 只有 Files 和 ResolvePlayback，尚未提供附属文件下载能力。
- `backend/internal/service/strm/generate_test.go:167` 输入带有海报图片，但现有测试只验收视频 STRM 和目录失败项，未要求图片落盘。
- 媒体库既有 strm_existing_file_policy 明确只适用于 STRM，不能未经讨论直接将覆盖语义推广到 NFO/图片。

## 规划边界

已作为父任务下第三个独立交付，在 SDK 改造之后接入，保持当前全量同步入口。用户已确认已有文件跳过；图片采用全部常见类型，但必须先有同目录视频对应的有效 STRM 才下载。最终合同以第三子任务 PRD/设计为准。

1. 按 GenerateNFO、GeneratePosterCover 开关同步网盘已存在的 .nfo 和对应图片，保持源目录层次及文件名。
2. 复用正式扫描清单收集附属文件，不为下载再次全库扫描；抽样预览不下载文件，仍遵守已确认四项预算。
3. SDK 负责可信文件取链、受限流控制的读取及可取消下载；同步层负责筛选、输出路径、文件冲突/归属、原子发布和任务结果。
4. 明确已有本地文件的处理、下载字节/时限/并发边界、失败与重跑行为，不能把半文件发布成成功产物。
5. 任务结果区分 STRM 写入与附属下载，并报告失败/跳过；关闭对应开关时不下载也不删除本地已有文件。
6. 下载远端现有文件，不扩大为 TMDB/Metatube 刮削补全或图片生成。图片沿用项目 image 后缀且不按命名过滤，字幕不在本轮范围。

## 状态

用户已同意建立第三子任务 `../11-25-library-sidecar-sync`，并确认已有文件和图片规则。PRD、设计、计划及证据已形成，父任务摘要已更新；尚未获实现授权，未开始下载实现。
