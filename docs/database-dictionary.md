# 115 Media Hub 精简数据库字段字典

日期：2026-10-25。状态：设计评审稿，尚未接入应用迁移或实现业务校验。

本版面向单人自用工具，使用 **17 张表、226 个字段**。以[SQL 草案](database-schema.sql)为结构依据，配套[设计说明](database-design.md)。表中 `_id` 均为代码管理的逻辑关联：数据库没有外键、级联删除或 CHECK；数据库保留主键、唯一性、非空、默认值与索引。时间为 UTC Unix 毫秒，外部 ID 为 TEXT，未知为 NULL。字典“必填”标记含 INTEGER PRIMARY KEY 的主键语义。

账号、密码哈希、签名密钥、管理会话与全局设置不存数据库。凭据、播放签名与管理会话策略见设计说明；外部连接授权凭据明文 JSON 保存在 connections；不包含在 API、日志或任务快照中。数据库与备份按敏感凭据保护。健康检测/资源锁保存在内存；图片文件、运行日志/审计 JSONL、备份 manifest 在磁盘；恢复门禁采用被恢复数据库之外的持久标记。

## 表目录

| 表 | 字段数 | 职责 |
|----|--------|------|
| `connections` | 14 | 外部连接与服务端凭据；首版仅一个活动 115。停用后可物理删除连接；有依赖时由代码阻止或先清理。 |
| `libraries` | 18 | 当前库配置；历史配置由运行与预览快照保存。状态仅为 enabled / disabled；安全条件满足时物理删除。 |
| `source_nodes` | 16 | 来源稳定身份及最新观察；路径变化不改变身份。 |
| `library_files` | 12 | 每库每来源一个永久播放 asset_id；来源、规则、归属分开。 |
| `strm_artifacts` | 15 | 受管 STRM 产物及路径占用；迁移先新后旧。 |
| `scan_directories` | 12 | 逐运行目录分页与一致性证据；递归完整性独立核验。 |
| `scan_entries` | 5 | 逐运行来源观察；独立行保存，避免 last_seen 推断删除。 |
| `operation_previews` | 11 | 预览头；明细保存在预览生成运行的 job_items。 |
| `job_plans` | 11 | 持久计划；enabled 用于停用，删除采用物理删除。 |
| `job_runs` | 25 | 持久运行；配置快照、九状态、取消与重启恢复。 |
| `job_items` | 15 | 预览逐项动作、执行和恢复结果；重试保留逻辑意图。 |
| `job_effects` | 15 | 外部副作用意图与核验账本；未知结果先核对再重试。 |
| `media_items` | 12 | 按库缓存媒体服务条目；图片索引与层级采用有界 JSON。 |
| `media_file_links` | 6 | 服务条目到实际稳定资产；支持多版本和分集。 |
| `metadata_files` | 17 | 真实伴随文件映射、当前观察和共同基线；不依赖可删除展示缓存。 |
| `metadata_decisions` | 10 | 内容指纹决议；另存目标独立于共同基线持久保存。 |
| `quarantine_records` | 13 | 隔离事实与保留期；恢复尝试写入任务项和副作用账本。 |

## connections

外部连接与服务端凭据；首版仅一个活动 115。停用后可物理删除连接；有依赖时由代码阻止或先清理。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 稳定连接命名空间；重新授权保持 ID |
| `kind` | TEXT | 是 | 无 | 115 / emby / jellyfin |
| `name` | TEXT | 是 | 无 | 连接显示名 |
| `external_account_id` | TEXT | 否 | 无 | 已验证账户或服务实例身份 |
| `api_base_url` | TEXT | 否 | 无 | 执行器访问地址 |
| `web_base_url` | TEXT | 否 | 无 | 浏览器打开服务地址 |
| `credential_json` | TEXT | 否 | 无 | 115 Cookie 凭据对象 JSON（UID/CID/SEID/KID）；服务端使用，不返回 API |
| `capabilities_json` | TEXT | 是 | 无 | 有 schema_version 的逐项能力、证据与检测时间；凭据变化时清空并重测 |
| `profile_json` | TEXT | 是 | '{}' | schema_version 与非敏感账号名、会员及容量快照；资料缺失时保持未知，不代表连接失效 |
| `is_active` | INTEGER | 是 | 0 | 活动标志 0/1 |
| `status` | TEXT | 是 | 'unchecked' | 连接摘要；不替代逐项能力证据 |
| `retry_after_at` | INTEGER | 否 | 无 | 适配器要求的最早重试时间 |
| `created_at` | INTEGER | 是 | 无 | 创建时间 |
| `updated_at` | INTEGER | 是 | 无 | 更新时间 |

数据库唯一性：`kind（部分条件见 SQL）`；`id`。

## libraries

当前库配置；历史配置由运行与预览快照保存。状态仅为 enabled / disabled；安全条件满足时物理删除。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 稳定库 ID |
| `name` | TEXT | 是 | 无 | 库名称 |
| `media_type` | TEXT | 是 | 无 | movie / series / other |
| `status` | TEXT | 是 | 'disabled' | enabled / disabled |
| `source_connection_id` | TEXT | 是 | 无 | 逻辑关联 connections.id；禁删仍被引用的连接 |
| `source_root_id` | TEXT | 是 | 无 | 来源稳定目录 ID |
| `source_path_snapshot` | TEXT | 是 | 无 | 来源展示路径；不作为身份键 |
| `output_root` | TEXT | 是 | 无 | 工具实际写入 STRM 的根 |
| `output_root_key` | TEXT | 是 | 无 | 归一化挂载/大小写路径键；代码另查嵌套冲突 |
| `service_connection_id` | TEXT | 否 | 无 | 逻辑关联 connections.id；当前媒体服务 |
| `service_library_external_id` | TEXT | 否 | 无 | 该服务原始媒体库 ID，不混同应用库 |
| `service_access_root` | TEXT | 否 | 无 | 媒体服务看到同批 STRM 的路径 |
| `metadata_read_root` | TEXT | 否 | 无 | 执行器读取实际 NFO/图片/字幕的根 |
| `config_json` | TEXT | 是 | 无 | schema_version、递归/命名/过滤/元数据白名单/稳定阈值/继承配置 |
| `config_hash` | TEXT | 是 | 无 | 规范化有效配置 SHA-256 |
| `automatic_cleanup` | INTEGER | 是 | 0 | 自动清理独立开关，默认关闭 |
| `created_at` | INTEGER | 是 | 无 | 创建时间 |
| `updated_at` | INTEGER | 是 | 无 | 更新时间 |

数据库唯一性：`id`。

## source_nodes

来源稳定身份及最新观察；路径变化不改变身份。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 本地稳定来源 ID |
| `connection_id` | TEXT | 是 | 无 | 逻辑关联 connections.id；禁止删除被引用连接 |
| `external_id` | TEXT | 是 | 无 | 远端稳定文件/目录 ID；不得转数字 |
| `parent_external_id` | TEXT | 否 | 无 | 当前父目录 ID |
| `kind` | TEXT | 是 | 无 | file / directory |
| `name` | TEXT | 是 | 无 | 当前文件名 |
| `extension` | TEXT | 否 | 无 | 小写扩展名 |
| `path_snapshot` | TEXT | 否 | 无 | 最近路径，仅展示 |
| `size_bytes` | INTEGER | 否 | 无 | 未知为 NULL |
| `remote_mtime` | INTEGER | 否 | 无 | 远端可获得修改时间 |
| `remote_version` | TEXT | 否 | 无 | 可获得的条件写入版本 |
| `remote_hash` | TEXT | 否 | 无 | 远端摘要 |
| `remote_hash_algorithm` | TEXT | 否 | 无 | 算法及语义必须可比较 |
| `identity_status` | TEXT | 是 | 'unverified' | verified / unverified；未验证不能推断移动/删除 |
| `last_seen_at` | INTEGER | 是 | 无 | 最新观察时间，不单独证明删除 |
| `created_at` | INTEGER | 是 | 无 | 首次登记时间 |

数据库唯一性：`connection_id + external_id`；`id`。

## library_files

每库每来源一个永久播放 asset_id；来源、规则、归属分开。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | asset_id；/p/:asset_id，不随路径或规则改变 |
| `library_id` | TEXT | 是 | 无 | 逻辑关联 libraries.id；物理删除前由代码清理或阻止残留依赖 |
| `source_node_id` | TEXT | 是 | 无 | 逻辑关联 source_nodes.id；被归属引用时禁删 |
| `source_status` | TEXT | 是 | 'unknown' | present / missing / moved_out / unknown |
| `rule_status` | TEXT | 是 | 'unknown' | included / excluded / unknown |
| `ownership_status` | TEXT | 是 | 'registered' | registered / unmatched / conflict |
| `evidence_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；缺失依据运行，证据被引用时禁删 |
| `evidence_json` | TEXT | 否 | 无 | schema_version、范围与完整目录证据；漏扫不等于缺失 |
| `last_successful_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；最近已核验处理 |
| `checked_at` | INTEGER | 否 | 无 | 核对时间 |
| `created_at` | INTEGER | 是 | 无 | 登记时间 |
| `updated_at` | INTEGER | 是 | 无 | 更新时间 |

数据库唯一性：`library_id + source_node_id`；`id`。

## strm_artifacts

受管 STRM 产物及路径占用；迁移先新后旧。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 产物 ID |
| `library_file_id` | TEXT | 是 | 无 | 逻辑关联 library_files.id；被产物引用时禁删 |
| `output_path` | TEXT | 是 | 无 | 工具实际输出路径 |
| `output_path_key` | TEXT | 是 | 无 | 全局规范化路径占用键 |
| `relative_path` | TEXT | 是 | 无 | 输出根内相对路径 |
| `lifecycle` | TEXT | 是 | 无 | planned / current / retired / quarantined |
| `path_reserved` | INTEGER | 是 | 1 | 旧路径确认移除前保持 1；retired 不自动释放 |
| `local_status` | TEXT | 是 | 'unchecked' | unchecked / generated / missing / needs_repair / user_modified |
| `last_written_hash` | TEXT | 否 | 无 | 最近工具写入 SHA-256；用户改写不可覆盖此依据 |
| `observed_hash` | TEXT | 否 | 无 | 最近实际内容 SHA-256 |
| `observed_size_bytes` | INTEGER | 否 | 无 | 未知为 NULL |
| `generated_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；生成/迁移运行 |
| `generated_at` | INTEGER | 否 | 无 | 最近工具成功写入时间 |
| `checked_at` | INTEGER | 否 | 无 | 最近本地核验时间 |
| `created_at` | INTEGER | 是 | 无 | 登记时间 |

数据库唯一性：`output_path_key（部分条件见 SQL）`；`library_file_id（部分条件见 SQL）`；`id`。

## scan_directories

逐运行目录分页与一致性证据；递归完整性独立核验。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 目录扫描 ID |
| `run_id` | TEXT | 是 | 无 | 逻辑关联 job_runs.id；被缺失/清理证据引用时禁删 |
| `directory_external_id` | TEXT | 是 | 无 | 目录稳定外部 ID |
| `parent_directory_external_id` | TEXT | 否 | 无 | 扫描树父目录 |
| `path_snapshot` | TEXT | 是 | 无 | 当时展示路径 |
| `status` | TEXT | 是 | 无 | pending / running / complete / failed / unknown |
| `pages_read` | INTEGER | 是 | 0 | 已读取页数 |
| `pagination_complete` | INTEGER | 是 | 0 | 确认分页终结 0/1 |
| `children_complete` | INTEGER | 是 | 0 | 所需子树覆盖 0/1 |
| `listing_consistent` | INTEGER | 否 | 无 | 1 一致、0 失败、NULL 未知 |
| `evidence_json` | TEXT | 否 | 无 | schema_version、前后版本/cursor/error/retry_after；无无界条目数组 |
| `completed_at` | INTEGER | 否 | 无 | 目录核验终结时间 |

数据库唯一性：`run_id + directory_external_id`；`id`。

## scan_entries

逐运行来源观察；独立行保存，避免 last_seen 推断删除。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `run_id`（PK） | TEXT | 是 | 无 | 逻辑关联 job_runs.id；与目录属于同一运行 |
| `source_node_id`（PK） | TEXT | 是 | 无 | 逻辑关联 source_nodes.id；身份独立于路径 |
| `directory_scan_id` | TEXT | 是 | 无 | 逻辑关联 scan_directories.id；证据被引用时禁删 |
| `observation_json` | TEXT | 是 | 无 | schema_version、当时名称/父级/路径/大小/摘要/版本的有界单条快照 |
| `observed_at` | INTEGER | 是 | 无 | 观察时间 |

数据库唯一性：`run_id + source_node_id`。

## operation_previews

预览头；明细保存在预览生成运行的 job_items。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 预览 ID |
| `library_id` | TEXT | 否 | 无 | 逻辑关联 libraries.id；全局恢复可 NULL |
| `operation` | TEXT | 是 | 无 | 受控操作类别 |
| `producer_run_id` | TEXT | 是 | 无 | 逻辑关联 job_runs.id；生产预览明细的运行 |
| `config_hash` | TEXT | 是 | 无 | 当前配置与候选配置规范化摘要 |
| `scope_json` | TEXT | 是 | 无 | schema_version、明确处理范围；大量选择通过 job_items 行保存 |
| `evidence_json` | TEXT | 是 | 无 | schema_version、配置摘要/来源条件写入版本/恢复包摘要与前置门禁 |
| `status` | TEXT | 是 | 无 | building / ready / blocked / expired / consumed / failed |
| `consumed_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；实际执行运行 |
| `expires_at` | INTEGER | 是 | 无 | 有效截止时间 |
| `created_at` | INTEGER | 是 | 无 | 生成时间 |

数据库唯一性：`consumed_run_id`；`producer_run_id`；`id`。

## job_plans

持久计划；enabled 用于停用，删除采用物理删除。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 计划 ID |
| `library_id` | TEXT | 是 | 无 | 逻辑关联 libraries.id；停用库时同步停用计划 |
| `name` | TEXT | 是 | 无 | 显示名称 |
| `operation` | TEXT | 是 | 无 | 受控操作类别 |
| `schedule_json` | TEXT | 是 | 无 | schema_version、interval/daily/weekly、秒数/HH:mm/weekday、IANA timezone |
| `parameters_json` | TEXT | 是 | 无 | schema_version、范围与参数模板，不含密钥 |
| `enabled` | INTEGER | 是 | 0 | 后续触发开关 |
| `next_run_at` | INTEGER | 否 | 无 | 下次时点 |
| `last_scheduled_at` | INTEGER | 否 | 无 | 最近触发时点；同计划时点幂等键去重 |
| `created_at` | INTEGER | 是 | 无 | 创建时间 |
| `updated_at` | INTEGER | 是 | 无 | 更新时间 |

数据库唯一性：`id`。

## job_runs

持久运行；配置快照、九状态、取消与重启恢复。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 运行 UUID |
| `display_no` | INTEGER | 是 | 无 | UI T-1042，短事务分配 |
| `library_id` | TEXT | 否 | 无 | 逻辑关联 libraries.id；全局维护允许 NULL |
| `plan_id` | TEXT | 否 | 无 | 逻辑关联 job_plans.id；手动运行可 NULL |
| `parent_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；重试/恢复来源 |
| `preview_id` | TEXT | 否 | 无 | 逻辑关联 operation_previews.id；预览执行关联 |
| `operation` | TEXT | 是 | 无 | 受控业务操作 |
| `trigger_kind` | TEXT | 是 | 无 | manual / schedule / retry / recovery / system |
| `request_key` | TEXT | 是 | 无 | 请求幂等键；调度含 plan+scheduled_at |
| `request_hash` | TEXT | 是 | 无 | 同键不同参数必须拒绝 |
| `config_snapshot_json` | TEXT | 是 | 无 | schema_version、有效库/全局配置/连接身份/路径/绑定；不含凭据 |
| `scope_snapshot_json` | TEXT | 是 | 无 | schema_version、启动范围及配置候选，不存无界任务明细 |
| `status` | TEXT | 是 | 无 | queued / running / retry_wait / cancelling / succeeded / partial / failed / cancelled / interrupted |
| `phase` | TEXT | 是 | 无 | scanning / comparing / writing / uploading / verifying 等 |
| `phase_total` | INTEGER | 否 | 无 | 未知为 NULL |
| `phase_done` | INTEGER | 是 | 0 | 当前阶段完成数 |
| `summary_json` | TEXT | 否 | 无 | schema_version、派生计数/扫描范围和覆盖结论/有限断点，不代替逐目录证据 |
| `attempt_count` | INTEGER | 是 | 0 | 有上限的尝试数 |
| `retry_limit` | INTEGER | 是 | 无 | 启动时重试上限 |
| `retry_after_at` | INTEGER | 否 | 无 | 最早重试时间 |
| `cancel_requested_at` | INTEGER | 否 | 无 | 持久取消请求 |
| `started_at` | INTEGER | 否 | 无 | 首次执行时间 |
| `finished_at` | INTEGER | 否 | 无 | 实际终结时间 |
| `created_at` | INTEGER | 是 | 无 | 排队时间 |
| `updated_at` | INTEGER | 是 | 无 | 状态更新时间 |

数据库唯一性：`plan_id（部分条件见 SQL）`；`request_key`；`display_no`；`id`。

## job_items

预览逐项动作、执行和恢复结果；重试保留逻辑意图。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 项 ID |
| `run_id` | TEXT | 是 | 无 | 逻辑关联 job_runs.id；保留追溯 |
| `parent_item_id` | TEXT | 否 | 无 | 逻辑关联 job_items.id；预览复制或重试来源 |
| `operation_id` | TEXT | 是 | 无 | 逻辑意图 ID；同操作重试复用，独立修复生成新值 |
| `target_kind` | TEXT | 是 | 无 | directory / asset / artifact / metadata / quarantine / backup 等 |
| `target_id` | TEXT | 是 | 无 | 稳定目标 ID；多态逻辑关联，由代码验证 |
| `action` | TEXT | 是 | 无 | 受控动作 |
| `status` | TEXT | 是 | 无 | pending / running / retry_wait / succeeded / skipped / failed / conflict / pending_verification / cancelled |
| `selected` | INTEGER | 是 | 0 | 预览中明确选定范围；执行复制选中项 |
| `input_json` | TEXT | 是 | 无 | schema_version、路径/远端条件写入版本/摘要/证据/阻断理由/预览动作，不含密钥 |
| `result_json` | TEXT | 否 | 无 | schema_version、实际结果/错误/核验和复用 effect ID；单项有界 |
| `attempt_count` | INTEGER | 是 | 0 | 项有限尝试次数 |
| `retry_after_at` | INTEGER | 否 | 无 | 最早重试时间 |
| `created_at` | INTEGER | 是 | 无 | 创建时间 |
| `updated_at` | INTEGER | 是 | 无 | 状态时间 |

数据库唯一性：`run_id + operation_id + target_kind + target_id + action`；`id`。

## job_effects

外部副作用意图与核验账本；未知结果先核对再重试。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 副作用 ID；同意图重试复用 |
| `item_id` | TEXT | 是 | 无 | 逻辑关联 job_items.id；原始意图项 |
| `operation_id` | TEXT | 是 | 无 | 稳定逻辑操作 ID；与原始项及复用结果一致 |
| `effect_key` | TEXT | 是 | 无 | operation+target+action+输入摘要+预期版本的幂等键 |
| `kind` | TEXT | 是 | 无 | local_write / rename / quarantine / upload / copy / restore / backup / service_scan |
| `target_key` | TEXT | 是 | 无 | 规范化输出或远端资源键 |
| `state` | TEXT | 是 | 无 | prepared / applied / unknown / verified / failed / compensated |
| `expected_json` | TEXT | 是 | 无 | schema_version、目标身份/内容/预期版本/前置条件 |
| `actual_json` | TEXT | 否 | 无 | schema_version、实际 ID/路径/摘要/响应关联/实际另存目标 |
| `recovery_json` | TEXT | 否 | 无 | schema_version、staging_path/backup_path/backup_hash/backup_verified_at/恢复策略 |
| `verification_json` | TEXT | 否 | 无 | schema_version、存在/大小/内容/条件写入核验，不确定保持未知 |
| `applied_at` | INTEGER | 否 | 无 | 外部操作发生时间 |
| `verified_at` | INTEGER | 否 | 无 | 事实核验时间 |
| `created_at` | INTEGER | 是 | 无 | 意图持久化时间 |
| `updated_at` | INTEGER | 是 | 无 | 状态时间 |

数据库唯一性：`effect_key`；`id`。

## media_items

按库缓存媒体服务条目；图片索引与层级采用有界 JSON。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 缓存条目 ID |
| `library_id` | TEXT | 是 | 无 | 逻辑关联 libraries.id；服务绑定变化时重建缓存 |
| `external_id` | TEXT | 是 | 无 | 服务原始条目 ID |
| `parent_item_id` | TEXT | 否 | 无 | 逻辑关联 media_items.id；代码清缓存时解除子级关联 |
| `kind` | TEXT | 是 | 无 | movie / series / season / episode / other |
| `title` | TEXT | 是 | 无 | 展示标题 |
| `sort_title` | TEXT | 是 | 无 | 排序名称 |
| `season_number` | INTEGER | 否 | 无 | 0 支持特典 |
| `episode_number` | INTEGER | 否 | 无 | 分集编号 |
| `details_json` | TEXT | 是 | 无 | schema_version、年份/评分/类型/演员/服务路径/provider IDs/图片引用，不存图片二进制 |
| `last_seen_at` | INTEGER | 是 | 无 | 最近成功刷新时间 |
| `unavailable_at` | INTEGER | 否 | 无 | 服务不可用/缓存过时时间，不据此清除离线海报 |

数据库唯一性：`library_id + external_id`；`id`。

## media_file_links

服务条目到实际稳定资产；支持多版本和分集。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `media_item_id`（PK） | TEXT | 是 | 无 | 逻辑关联 media_items.id；清缓存时由代码显式清理链接 |
| `library_file_id`（PK） | TEXT | 是 | 无 | 逻辑关联 library_files.id；同库/绑定/来源必须核验 |
| `service_path` | TEXT | 否 | 无 | 服务报告路径；不作为来源身份 |
| `match_status` | TEXT | 是 | 无 | verified / pending / ambiguous |
| `evidence_json` | TEXT | 是 | 无 | schema_version、路径/服务绑定/版本/归属证据 |
| `updated_at` | INTEGER | 是 | 无 | 核对时间 |

数据库唯一性：`media_item_id + library_file_id`。

## metadata_files

真实伴随文件映射、当前观察和共同基线；不依赖可删除展示缓存。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 稳定元数据文件 ID |
| `library_id` | TEXT | 是 | 无 | 逻辑关联 libraries.id；删除库前须处理该库的持久化元数据记录 |
| `owner_source_node_id` | TEXT | 否 | 无 | 逻辑关联 source_nodes.id；电影/剧/季/集/版本持久锚点 |
| `library_file_id` | TEXT | 否 | 无 | 逻辑关联 library_files.id；具体视频/分集；通用剧季图可 NULL |
| `file_kind` | TEXT | 是 | 无 | nfo / image / subtitle |
| `local_path` | TEXT | 是 | 无 | metadata_read_root 下实际伴随文件路径 |
| `local_path_key` | TEXT | 是 | 无 | 规范化本地路径键 |
| `target_connection_id` | TEXT | 是 | 无 | 逻辑关联 connections.id；115 目标命名空间 |
| `target_key` | TEXT | 否 | 无 | 连接+目标父目录 ID+规范化文件名；未匹配可 NULL |
| `mapping_status` | TEXT | 是 | 无 | verified / unmatched / ambiguous / conflict |
| `mapping_json` | TEXT | 是 | 无 | schema_version、owner_level/父目录外部 ID/原文件名/STRM 依据/反向命名证据 |
| `observation_json` | TEXT | 否 | 无 | schema_version、两次完整本地读取、远端事实、可比性和比较时基线摘要 |
| `baseline_json` | TEXT | 否 | 无 | schema_version、双方已核验相同的 target_key/摘要/算法/大小/远端身份版本/effect/verified_at |
| `comparison` | TEXT | 是 | 'unknown' | new / same / local_changed / remote_changed / both_changed / no_baseline_conflict / unknown / local_missing / waiting_stable / invalid |
| `observed_run_id` | TEXT | 否 | 无 | 逻辑关联 job_runs.id；当前观察来源；历史事实存对应 job_items/effects |
| `created_at` | INTEGER | 是 | 无 | 登记时间 |
| `updated_at` | INTEGER | 是 | 无 | 核对时间 |

数据库唯一性：`library_id + local_path_key`；`id`。

## metadata_decisions

内容指纹决议；另存目标独立于共同基线持久保存。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 决议 ID |
| `metadata_file_id` | TEXT | 是 | 无 | 逻辑关联 metadata_files.id；有决议/基线证据时禁删 |
| `fingerprint` | TEXT | 是 | 无 | 映射+目标身份+双方可比内容摘要+基线摘要；同组合去重 |
| `decision` | TEXT | 是 | 无 | keep_remote / save_copy / replace_local |
| `state` | TEXT | 是 | 无 | requested / blocked / applied / verified / obsolete |
| `snapshot_json` | TEXT | 是 | 无 | schema_version、决议时双方/目标/基线/预期版本不可变快照 |
| `actual_target_json` | TEXT | 否 | 无 | schema_version、实际另存名称/ID/路径/内容及核验；同指纹复用 |
| `effect_id` | TEXT | 否 | 无 | 逻辑关联 job_effects.id；已执行效果及可恢复备份 |
| `created_at` | INTEGER | 是 | 无 | 决议时间 |
| `updated_at` | INTEGER | 是 | 无 | 状态时间 |

数据库唯一性：`fingerprint`；`id`。

## quarantine_records

隔离事实与保留期；恢复尝试写入任务项和副作用账本。

| 字段 | 类型 | 必填 | 默认值 | 含义及逻辑关联 |
| --- | --- | --- | --- | --- |
| `id`（PK） | TEXT | 是 | 无 | 隔离记录 ID |
| `artifact_id` | TEXT | 是 | 无 | 逻辑关联 strm_artifacts.id；原受管产物 |
| `quarantine_effect_id` | TEXT | 是 | 无 | 逻辑关联 job_effects.id；隔离执行及核验 |
| `original_path` | TEXT | 是 | 无 | 原始实际路径 |
| `quarantine_path` | TEXT | 是 | 无 | 扫描根之外隔离位置 |
| `content_hash` | TEXT | 是 | 无 | 隔离内容 SHA-256 |
| `size_bytes` | INTEGER | 是 | 无 | 已核验大小 |
| `evidence_json` | TEXT | 是 | 无 | schema_version、完整扫描/范围/原文件未修改/原路径键证据 |
| `status` | TEXT | 是 | 无 | pending / quarantined / restored / expired / purged / conflict |
| `expires_at` | INTEGER | 是 | 无 | 保留截止时间；不等于已经物理删除 |
| `last_restore_item_id` | TEXT | 否 | 无 | 逻辑关联 job_items.id；最新恢复尝试；完整历史在任务项/effect |
| `created_at` | INTEGER | 是 | 无 | 登记时间 |
| `updated_at` | INTEGER | 是 | 无 | 实际状态时间 |

数据库唯一性：`quarantine_path`；`quarantine_effect_id`；`id`。

## 代码层契约（待实现）

所有 JSON 都必须经明确 Go 类型解析，包含 `schema_version`，并拒绝未知/不支持的结构版本、非法值和不适用字段。默认空对象不代表核验成功。JSON 仅用于单对象配置、快照与有界证据；扫描条目、任务项、演员/图片等输入须有边界，任务/扫描集合必须分页行存储，不塞进一个无限增长数组。

| 范围 | Service 必须保证 |
| --- | --- |
| 基础值 | UUID/外部 ID 非空、布尔为 0/1、字节/计数非负、、合法 URL/时区/HH:mm；显式区分 NULL 未知与 0，枚举与转换采用白名单。 |
| 关联与删除 | 同一短事务检查逻辑目标存在、同库/同连接归属与写入条件；不设逻辑删除字段；enabled/is_active 用于停用。代码检查持久依赖后才物理删除连接或库；被持久业务记录引用时阻止删除。删除计划时将历史运行的 plan_id 置空，运行快照保留执行事实；可重建缓存关系先清理。迁移/导入/备份恢复/维护脚本遵循相同契约。 |
| 路径与归属 | 来源根、工具输出根、媒体服务访问根和元数据读取根角色分开；staging/隔离在扫描根外。规范化含大小写/挂载/符号链接，并检查同根及父子包含。稳定 asset_id 不随改名/移动/重试改变，规则排除不等于来源删除。 |
| 部分唯一索引 | kind/is_active、lifecycle/path_reserved、plan_id/status 的值域和转换须先校验。planned/current 必须保留路径占用；旧产物 retired 仍占路径，确认移走/删除后才释放。非法枚举或布尔不得绕开索引条件。 |
| 连接凭据与能力 | credential_json 保存明文 115 Cookie 凭据 JSON；禁凭据进入 API、日志/任务快照/URL。数据库和备份按敏感凭据保护。capabilities_json 含逐项 status/evidence/checked_at；凭据变化后清空并重新检测。重新授权沿用连接 ID，账户身份变化必须阻断复用。 |
| 配置与预览 | 单实例内串行更新当前配置；有效配置与候选配置/全局值/路径/绑定写入不可变任务快照。预览 detail 在 producer_run 的 job_items；执行在同一短事务验证配置/证据/期限/明确选项，消费预览并复制选中项，以 parent_item_id 追溯；首次执行分配新 operation_id，同操作失败重试继承它。恢复预览须校验磁盘 manifest 与恢复门禁。 |
| 扫描与缺失 | 目录 complete 至少要求 pagination_complete=1 且 listing_consistent=1；子树缺失证明另要求 children_complete=1 及全部所需目录证据，同次 scan_entries/目录/run/配置范围一致。漏页、鉴权失败、一致性未知/失败、骤减、未验证稳定身份均阻断删除；last_seen 或条目不在当前批次不单独证明 missing。 |
| 任务与恢复 | job_runs 仅九状态；核验是 phase。单实例进程锁在外部持有；内存锁串行协调目标，启动时将未终结任务转 interrupted 并核对 effects 后再领取，避免双重执行。取消请求持久化，外部 I/O 不跨事务；幂等请求同键不同参数拒绝、重试有界、失败项单独复制。 |
| 外部副作用 | prepared 先落盘再调用外部系统；不确定结果记 unknown/pending_verification，先查实际结果再决定重传。effect_key 包含稳定 operation_id；同逻辑意图重试复用效果，新修复意图新 ID。恢复/备份/另存结果、备份摘要和已核验时间写 recovery/actual/verification JSON；verified 必须有足够事实。内存锁不能防止第三方修改，覆盖仍须摘要/远端版本条件核验。 |
| 媒体缓存 | media_items 属于当前库服务绑定；服务绑定变更有控制地清缓存，连接离线保留缓存和磁盘图片。details_json 图片仅存缓存路径/索引/状态，层级不得形成环；链接核验到具体 asset_id/分集/版本，同名标题不能证明归属。 |
| 元数据映射 | mapping_json 类型含 owner_level、target_parent_external_id、target_name、artifact_id、反向命名证据；verified 必须有稳定 owner_source_node_id 与完整目标身份。电影/剧/季/集/多版本/特典独立识别，不能用可删除媒体缓存 ID 作唯一锚点。 |
| 当前观察 | observation_json 类型含两次本地 {hash,size,mtime,checked_at}、稳定间隔与完整可读/内容解析 validation，以及远端 {state,external_id,hash,hash_algorithm,size,version,checked_at}、比较时 baseline 摘要与证据。local_state 区分 missing/unreadable/changing/stable/invalid/unknown，remote_state 区分 present/missing/unknown；未知不可当 missing，同算法同语义摘要才比较。更新当前观察不改写任务不可变快照。 |
| 共同基线 | baseline_json 类型含 schema_version、target_key、content_hash、hash_algorithm、size_bytes、remote_external_id/remote_version、provenance、verified_effect_id、verified_at；仅 verified_upload 或 verified_equal 更新。目标/绑定变化使旧基线失效；比较为同/本地改/远端改/双方改/无基线冲突/未知，不能只比较时间或大小。keep_remote、save_copy 不能伪造原目标的共同基线。 |
| 冲突决议 | 决议 snapshot 保存不可变双方/目标/基线；fingerprint 绑定内容及目标。相同 fingerprint 的 save_copy 复用 actual_target/effect，不能重复另存；内容、目标或版本变化重新核验并使过期决议 obsolete。覆盖前验证目标版本、内容及可恢复备份，不支持可靠条件写入时禁止自动原位覆盖。 |
| 隔离与恢复 | 隔离前证明完整扫描、范围、工具归属及用户未改写，核验后才记 quarantined。到期仅为候选，实际清除核验后才记 purged。恢复冲突不得覆盖现有文件；每次恢复是 job_item/effect，记录用户选择/实际路径/备份及核验，保留历史。恢复数据库外门禁核对完成并由用户明确放行后才启用自动清理/覆盖。 |

以上是设计契约，不代表已实现 Service 校验、任务运行器、会话管理、恢复或真实服务接入。本版替代旧 39 表/515 字段方案；旧 143 条 CHECK 清单随拆分模型废止，规则按此精简模型重新组织。

