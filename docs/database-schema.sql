-- 115 Media Hub 精简数据库设计草案，2026-10-25。
-- 仅供设计评审与临时 SQLite 验证；不由应用加载，不是 goose migration。
-- 不声明外键或 CHECK；关联、删除策略、JSON/值域/状态/单例由 Service 校验。
-- PRIMARY KEY / UNIQUE / NOT NULL / 默认值及索引由数据库保留。
-- TEXT UUID 为应用主键；外部 ID 使用 TEXT；时间为 UTC Unix 毫秒；未知值使用 NULL。
-- 账号/密码/会话/全局设置/健康/锁/图片文件/日志/备份清单/恢复门禁均不入本数据库。

-- @table connections | 外部连接与服务端凭据；首版仅一个活动 115。
CREATE TABLE connections (
  id TEXT PRIMARY KEY NOT NULL, -- 稳定连接命名空间；重新授权保持 ID
  kind TEXT NOT NULL, -- 115 / emby / jellyfin
  name TEXT NOT NULL, -- 连接显示名
  external_account_id TEXT, -- 已验证账户或服务实例身份
  api_base_url TEXT, -- 执行器访问地址
  web_base_url TEXT, -- 浏览器打开服务地址
  credential_json TEXT, -- 115 Cookie 凭据 JSON（UID/CID/SEID/KID）；服务端使用
  capabilities_json TEXT NOT NULL, -- 有 schema_version 的逐项能力、证据与检测时间；凭据变化时清空并重测
  profile_json TEXT NOT NULL DEFAULT '{}', -- schema_version 与非敏感账号、会员、容量快照；缺失资料用 NULL/未知
  is_active INTEGER NOT NULL DEFAULT 0, -- 活动标志 0/1
  status TEXT NOT NULL DEFAULT 'unchecked', -- 连接摘要；不替代逐项能力证据
  retry_after_at INTEGER, -- 适配器要求的最早重试时间
  created_at INTEGER NOT NULL, -- 创建时间
  updated_at INTEGER NOT NULL -- 更新时间
);
CREATE UNIQUE INDEX uq_active_drive ON connections(kind) WHERE kind='115' AND is_active=1;

-- @table libraries | 当前库配置；历史配置由运行与预览快照保存。
CREATE TABLE libraries (
  id TEXT PRIMARY KEY NOT NULL, -- 稳定库 ID
  name TEXT NOT NULL, -- 库名称
  media_type TEXT NOT NULL, -- movie / series / other
  status TEXT NOT NULL DEFAULT 'disabled', -- enabled / disabled
  source_connection_id TEXT NOT NULL, -- 逻辑关联 connections.id；禁删仍被引用的连接
  source_root_id TEXT NOT NULL, -- 来源稳定目录 ID
  source_path_snapshot TEXT NOT NULL, -- 来源展示路径；不作为身份键
  output_root TEXT NOT NULL, -- 工具实际写入 STRM 的根
  output_root_key TEXT NOT NULL, -- 归一化挂载/大小写路径键；代码另查嵌套冲突
  service_connection_id TEXT, -- 逻辑关联 connections.id；当前媒体服务
  service_library_external_id TEXT, -- 该服务原始媒体库 ID，不混同应用库
  service_access_root TEXT, -- 媒体服务看到同批 STRM 的路径
  metadata_read_root TEXT, -- 执行器读取实际 NFO/图片/字幕的根
  config_json TEXT NOT NULL, -- schema_version、递归/命名/过滤/元数据白名单/稳定阈值/继承配置
  config_hash TEXT NOT NULL, -- 规范化有效配置 SHA-256
  automatic_cleanup INTEGER NOT NULL DEFAULT 0, -- 自动清理独立开关，默认关闭
  created_at INTEGER NOT NULL, -- 创建时间
  updated_at INTEGER NOT NULL -- 更新时间
);
CREATE INDEX idx_library_connection ON libraries(source_connection_id, status);

-- @table source_nodes | 来源稳定身份及最新观察；路径变化不改变身份。
CREATE TABLE source_nodes (
  id TEXT PRIMARY KEY NOT NULL, -- 本地稳定来源 ID
  connection_id TEXT NOT NULL, -- 逻辑关联 connections.id；禁止删除被引用连接
  external_id TEXT NOT NULL, -- 远端稳定文件/目录 ID；不得转数字
  parent_external_id TEXT, -- 当前父目录 ID
  kind TEXT NOT NULL, -- file / directory
  name TEXT NOT NULL, -- 当前文件名
  extension TEXT, -- 小写扩展名
  path_snapshot TEXT, -- 最近路径，仅展示
  size_bytes INTEGER, -- 未知为 NULL
  remote_mtime INTEGER, -- 远端可获得修改时间
  remote_version TEXT, -- 可获得的条件写入版本
  remote_hash TEXT, -- 远端摘要
  remote_hash_algorithm TEXT, -- 算法及语义必须可比较
  identity_status TEXT NOT NULL DEFAULT 'unverified', -- verified / unverified；未验证不能推断移动/删除
  last_seen_at INTEGER NOT NULL, -- 最新观察时间，不单独证明删除
  created_at INTEGER NOT NULL, -- 首次登记时间
  UNIQUE (connection_id, external_id)
);
CREATE INDEX idx_source_parent_name ON source_nodes(connection_id,parent_external_id,kind,name,id);
CREATE INDEX idx_source_parent_size ON source_nodes(connection_id,parent_external_id,size_bytes,id);
CREATE INDEX idx_source_parent_mtime ON source_nodes(connection_id,parent_external_id,remote_mtime,id);

-- @table library_files | 每库每来源一个永久播放 asset_id；来源、规则、归属分开。
CREATE TABLE library_files (
  id TEXT PRIMARY KEY NOT NULL, -- asset_id；/p/:asset_id，不随路径或规则改变
  library_id TEXT NOT NULL, -- 逻辑关联 libraries.id；物理删除前由代码清理或阻止残留依赖
  source_node_id TEXT NOT NULL, -- 逻辑关联 source_nodes.id；被归属引用时禁删
  source_status TEXT NOT NULL DEFAULT 'unknown', -- present / missing / moved_out / unknown
  rule_status TEXT NOT NULL DEFAULT 'unknown', -- included / excluded / unknown
  ownership_status TEXT NOT NULL DEFAULT 'registered', -- registered / unmatched / conflict
  evidence_run_id TEXT, -- 逻辑关联 job_runs.id；缺失依据运行，证据被引用时禁删
  evidence_json TEXT, -- schema_version、范围与完整目录证据；漏扫不等于缺失
  last_successful_run_id TEXT, -- 逻辑关联 job_runs.id；最近已核验处理
  checked_at INTEGER, -- 核对时间
  created_at INTEGER NOT NULL, -- 登记时间
  updated_at INTEGER NOT NULL, -- 更新时间
  UNIQUE (library_id,source_node_id)
);
CREATE INDEX idx_library_file_state ON library_files(library_id,source_status,rule_status,id);
CREATE INDEX idx_library_file_source ON library_files(source_node_id);

-- @table strm_artifacts | 受管 STRM 产物及路径占用；移动先新后旧。
CREATE TABLE strm_artifacts (
  id TEXT PRIMARY KEY NOT NULL, -- 产物 ID
  library_file_id TEXT NOT NULL, -- 逻辑关联 library_files.id；被产物引用时禁删
  output_path TEXT NOT NULL, -- 工具实际输出路径
  output_path_key TEXT NOT NULL, -- 全局规范化路径占用键
  relative_path TEXT NOT NULL, -- 输出根内相对路径
  lifecycle TEXT NOT NULL, -- planned / current / retired / quarantined
  path_reserved INTEGER NOT NULL DEFAULT 1, -- 旧路径确认移除前保持 1；retired 不自动释放
  local_status TEXT NOT NULL DEFAULT 'unchecked', -- unchecked / generated / missing / needs_repair / user_modified
  last_written_hash TEXT, -- 最近工具写入 SHA-256；用户改写不可覆盖此依据
  observed_hash TEXT, -- 最近实际内容 SHA-256
  observed_size_bytes INTEGER, -- 未知为 NULL
  generated_run_id TEXT, -- 逻辑关联 job_runs.id；生成/迁移运行
  generated_at INTEGER, -- 最近工具成功写入时间
  checked_at INTEGER, -- 最近本地核验时间
  created_at INTEGER NOT NULL -- 登记时间
);
CREATE UNIQUE INDEX uq_current_artifact ON strm_artifacts(library_file_id) WHERE lifecycle='current';
CREATE UNIQUE INDEX uq_reserved_output ON strm_artifacts(output_path_key) WHERE path_reserved=1;
CREATE INDEX idx_artifact_file ON strm_artifacts(library_file_id,local_status);

-- @table scan_directories | 逐运行目录分页与一致性证据；递归完整性独立核验。
CREATE TABLE scan_directories (
  id TEXT PRIMARY KEY NOT NULL, -- 目录扫描 ID
  run_id TEXT NOT NULL, -- 逻辑关联 job_runs.id；被缺失/清理证据引用时禁删
  directory_external_id TEXT NOT NULL, -- 目录稳定外部 ID
  parent_directory_external_id TEXT, -- 扫描树父目录
  path_snapshot TEXT NOT NULL, -- 当时展示路径
  status TEXT NOT NULL, -- pending / running / complete / failed / unknown
  pages_read INTEGER NOT NULL DEFAULT 0, -- 已读取页数
  pagination_complete INTEGER NOT NULL DEFAULT 0, -- 确认分页终结 0/1
  children_complete INTEGER NOT NULL DEFAULT 0, -- 所需子树覆盖 0/1
  listing_consistent INTEGER, -- 1 一致、0 失败、NULL 未知
  evidence_json TEXT, -- schema_version、前后版本/cursor/error/retry_after；无无界条目数组
  completed_at INTEGER, -- 目录核验终结时间
  UNIQUE (run_id,directory_external_id)
);
CREATE INDEX idx_scan_directory_run ON scan_directories(run_id,status,id);

-- @table scan_entries | 逐运行来源观察；独立行保存，避免 last_seen 推断删除。
CREATE TABLE scan_entries (
  run_id TEXT NOT NULL, -- 逻辑关联 job_runs.id；与目录属于同一运行
  source_node_id TEXT NOT NULL, -- 逻辑关联 source_nodes.id；身份独立于路径
  directory_scan_id TEXT NOT NULL, -- 逻辑关联 scan_directories.id；证据被引用时禁删
  observation_json TEXT NOT NULL, -- schema_version、当时名称/父级/路径/大小/摘要/版本的有界单条快照
  observed_at INTEGER NOT NULL, -- 观察时间
  PRIMARY KEY (run_id,source_node_id)
);
CREATE INDEX idx_scan_entry_directory ON scan_entries(directory_scan_id,source_node_id);

-- @table operation_previews | 预览头；明细保存在预览生成运行的 job_items。
CREATE TABLE operation_previews (
  id TEXT PRIMARY KEY NOT NULL, -- 预览 ID
  library_id TEXT, -- 逻辑关联 libraries.id；全局恢复可 NULL
  operation TEXT NOT NULL, -- 受控操作类别
  producer_run_id TEXT NOT NULL UNIQUE, -- 逻辑关联 job_runs.id；生产预览明细的运行
  config_hash TEXT NOT NULL, -- 当前配置与候选配置规范化摘要
  scope_json TEXT NOT NULL, -- schema_version、明确处理范围；大量选择通过 job_items 行保存
  evidence_json TEXT NOT NULL, -- schema_version、配置摘要/来源条件写入版本/恢复包摘要与前置门禁
  status TEXT NOT NULL, -- building / ready / blocked / expired / consumed / failed
  consumed_run_id TEXT UNIQUE, -- 逻辑关联 job_runs.id；实际执行运行
  expires_at INTEGER NOT NULL, -- 有效截止时间
  created_at INTEGER NOT NULL -- 生成时间
);
CREATE INDEX idx_preview_library ON operation_previews(library_id,created_at,id);

-- @table job_plans | 持久计划；停用使用 enabled，删除采用物理删除。
CREATE TABLE job_plans (
  id TEXT PRIMARY KEY NOT NULL, -- 计划 ID
  library_id TEXT NOT NULL, -- 逻辑关联 libraries.id；停用库时同步停用计划
  name TEXT NOT NULL, -- 显示名称
  operation TEXT NOT NULL, -- 受控操作类别
  schedule_json TEXT NOT NULL, -- schema_version、interval/daily/weekly、秒数/HH:mm/weekday、IANA timezone
  parameters_json TEXT NOT NULL, -- schema_version、范围与参数模板，不含密钥
  enabled INTEGER NOT NULL DEFAULT 0, -- 后续触发开关
  next_run_at INTEGER, -- 下次时点
  last_scheduled_at INTEGER, -- 最近触发时点；同计划时点幂等键去重
  created_at INTEGER NOT NULL, -- 创建时间
  updated_at INTEGER NOT NULL -- 更新时间
);
CREATE INDEX idx_plan_due ON job_plans(next_run_at) WHERE enabled=1;
CREATE INDEX idx_plan_library ON job_plans(library_id);

-- @table job_runs | 持久运行；配置快照、九状态、取消与重启恢复。
CREATE TABLE job_runs (
  id TEXT PRIMARY KEY NOT NULL, -- 运行 UUID
  display_no INTEGER NOT NULL UNIQUE, -- UI T-1042，短事务分配
  library_id TEXT, -- 逻辑关联 libraries.id；全局维护允许 NULL
  plan_id TEXT, -- 逻辑关联 job_plans.id；手动运行可 NULL
  parent_run_id TEXT, -- 逻辑关联 job_runs.id；重试/恢复来源
  preview_id TEXT, -- 逻辑关联 operation_previews.id；预览执行关联
  operation TEXT NOT NULL, -- 受控业务操作
  trigger_kind TEXT NOT NULL, -- manual / schedule / retry / recovery / system
  request_key TEXT NOT NULL UNIQUE, -- 请求幂等键；调度含 plan+scheduled_at
  request_hash TEXT NOT NULL, -- 同键不同参数必须拒绝
  config_snapshot_json TEXT NOT NULL, -- schema_version、有效库/全局配置/连接身份/路径/绑定；不含凭据
  scope_snapshot_json TEXT NOT NULL, -- schema_version、启动范围及配置候选，不存无界任务明细
  status TEXT NOT NULL, -- queued / running / retry_wait / cancelling / succeeded / partial / failed / cancelled / interrupted
  phase TEXT NOT NULL, -- scanning / comparing / writing / uploading / verifying 等
  phase_total INTEGER, -- 未知为 NULL
  phase_done INTEGER NOT NULL DEFAULT 0, -- 当前阶段完成数
  summary_json TEXT, -- schema_version、派生计数/扫描范围和覆盖结论/有限断点，不代替逐目录证据
  attempt_count INTEGER NOT NULL DEFAULT 0, -- 有上限的尝试数
  retry_limit INTEGER NOT NULL, -- 启动时重试上限
  retry_after_at INTEGER, -- 最早重试时间
  cancel_requested_at INTEGER, -- 持久取消请求
  started_at INTEGER, -- 首次执行时间
  finished_at INTEGER, -- 实际终结时间
  created_at INTEGER NOT NULL, -- 排队时间
  updated_at INTEGER NOT NULL -- 状态更新时间
);
CREATE UNIQUE INDEX uq_plan_queued ON job_runs(plan_id) WHERE plan_id IS NOT NULL AND status='queued';
CREATE INDEX idx_run_claim ON job_runs(status,retry_after_at,created_at);
CREATE INDEX idx_run_library ON job_runs(library_id,created_at,id);

-- @table job_items | 预览逐项动作、执行和恢复结果；重试保留逻辑意图。
CREATE TABLE job_items (
  id TEXT PRIMARY KEY NOT NULL, -- 项 ID
  run_id TEXT NOT NULL, -- 逻辑关联 job_runs.id；保留追溯
  parent_item_id TEXT, -- 逻辑关联 job_items.id；预览复制或重试来源
  operation_id TEXT NOT NULL, -- 逻辑意图 ID；同操作重试复用，独立修复生成新值
  target_kind TEXT NOT NULL, -- directory / asset / artifact / metadata / quarantine / backup 等
  target_id TEXT NOT NULL, -- 稳定目标 ID；多态逻辑关联，由代码验证
  action TEXT NOT NULL, -- 受控动作
  status TEXT NOT NULL, -- pending / running / retry_wait / succeeded / skipped / failed / conflict / pending_verification / cancelled
  selected INTEGER NOT NULL DEFAULT 0, -- 预览中明确选定范围；执行复制选中项
  input_json TEXT NOT NULL, -- schema_version、路径/远端条件写入版本/摘要/证据/阻断理由/预览动作，不含密钥
  result_json TEXT, -- schema_version、实际结果/错误/核验和复用 effect ID；单项有界
  attempt_count INTEGER NOT NULL DEFAULT 0, -- 项有限尝试次数
  retry_after_at INTEGER, -- 最早重试时间
  created_at INTEGER NOT NULL, -- 创建时间
  updated_at INTEGER NOT NULL, -- 状态时间
  UNIQUE (run_id,operation_id,target_kind,target_id,action)
);
CREATE INDEX idx_item_run_status ON job_items(run_id,status,id);
CREATE INDEX idx_item_parent ON job_items(parent_item_id);

-- @table job_effects | 外部副作用意图与核验账本；未知结果先核对再重试。
CREATE TABLE job_effects (
  id TEXT PRIMARY KEY NOT NULL, -- 副作用 ID；同意图重试复用
  item_id TEXT NOT NULL, -- 逻辑关联 job_items.id；原始意图项
  operation_id TEXT NOT NULL, -- 稳定逻辑操作 ID；与原始项及复用结果一致
  effect_key TEXT NOT NULL UNIQUE, -- operation+target+action+输入摘要+预期版本的幂等键
  kind TEXT NOT NULL, -- local_write / rename / quarantine / upload / copy / restore / backup / service_scan
  target_key TEXT NOT NULL, -- 规范化输出或远端资源键
  state TEXT NOT NULL, -- prepared / applied / unknown / verified / failed / compensated
  expected_json TEXT NOT NULL, -- schema_version、目标身份/内容/预期版本/前置条件
  actual_json TEXT, -- schema_version、实际 ID/路径/摘要/响应关联/实际另存目标
  recovery_json TEXT, -- schema_version、staging_path/backup_path/backup_hash/backup_verified_at/恢复策略
  verification_json TEXT, -- schema_version、存在/大小/内容/条件写入核验，不确定保持未知
  applied_at INTEGER, -- 外部操作发生时间
  verified_at INTEGER, -- 事实核验时间
  created_at INTEGER NOT NULL, -- 意图持久化时间
  updated_at INTEGER NOT NULL -- 状态时间
);
CREATE INDEX idx_effect_recovery ON job_effects(state,updated_at);
CREATE INDEX idx_effect_item ON job_effects(item_id);
CREATE INDEX idx_effect_operation ON job_effects(operation_id,target_key);

-- @table media_items | 按库缓存媒体服务条目；图片索引与层级采用有界 JSON。
CREATE TABLE media_items (
  id TEXT PRIMARY KEY NOT NULL, -- 缓存条目 ID
  library_id TEXT NOT NULL, -- 逻辑关联 libraries.id；服务绑定变化时重建缓存
  external_id TEXT NOT NULL, -- 服务原始条目 ID
  parent_item_id TEXT, -- 逻辑关联 media_items.id；代码清缓存时解除子级关联
  kind TEXT NOT NULL, -- movie / series / season / episode / other
  title TEXT NOT NULL, -- 展示标题
  sort_title TEXT NOT NULL, -- 排序名称
  season_number INTEGER, -- 0 支持特典
  episode_number INTEGER, -- 分集编号
  details_json TEXT NOT NULL, -- schema_version、年份/评分/类型/演员/服务路径/provider IDs/图片引用，不存图片二进制
  last_seen_at INTEGER NOT NULL, -- 最近成功刷新时间
  unavailable_at INTEGER, -- 服务不可用/缓存过时时间，不据此清除离线海报
  UNIQUE (library_id,external_id)
);
CREATE INDEX idx_media_wall ON media_items(library_id,kind,sort_title,id);
CREATE INDEX idx_media_children ON media_items(parent_item_id,season_number,episode_number,id);

-- @table media_file_links | 服务条目到实际稳定资产；支持多版本和分集。
CREATE TABLE media_file_links (
  media_item_id TEXT NOT NULL, -- 逻辑关联 media_items.id；清缓存时由代码显式清理链接
  library_file_id TEXT NOT NULL, -- 逻辑关联 library_files.id；同库/绑定/来源必须核验
  service_path TEXT, -- 服务报告路径；不作为来源身份
  match_status TEXT NOT NULL, -- verified / pending / ambiguous
  evidence_json TEXT NOT NULL, -- schema_version、路径/服务绑定/版本/归属证据
  updated_at INTEGER NOT NULL, -- 核对时间
  PRIMARY KEY (media_item_id,library_file_id)
);
CREATE INDEX idx_media_link_asset ON media_file_links(library_file_id);

-- @table metadata_files | 真实伴随文件映射、当前观察和共同基线；不依赖可重建展示缓存。
CREATE TABLE metadata_files (
  id TEXT PRIMARY KEY NOT NULL, -- 稳定元数据文件 ID
  library_id TEXT NOT NULL, -- 逻辑关联 libraries.id；删除库前须处理该库的持久化元数据记录
  owner_source_node_id TEXT, -- 逻辑关联 source_nodes.id；电影/剧/季/集/版本持久锚点
  library_file_id TEXT, -- 逻辑关联 library_files.id；具体视频/分集；通用剧季图可 NULL
  file_kind TEXT NOT NULL, -- nfo / image / subtitle
  local_path TEXT NOT NULL, -- metadata_read_root 下实际伴随文件路径
  local_path_key TEXT NOT NULL, -- 规范化本地路径键
  target_connection_id TEXT NOT NULL, -- 逻辑关联 connections.id；115 目标命名空间
  target_key TEXT, -- 连接+目标父目录 ID+规范化文件名；未匹配可 NULL
  mapping_status TEXT NOT NULL, -- verified / unmatched / ambiguous / conflict
  mapping_json TEXT NOT NULL, -- schema_version、owner_level/父目录外部 ID/原文件名/STRM 依据/反向命名证据
  observation_json TEXT, -- schema_version、两次完整本地读取、远端事实、可比性和比较时基线摘要
  baseline_json TEXT, -- schema_version、双方已核验相同的 target_key/摘要/算法/大小/远端身份版本/effect/verified_at
  comparison TEXT NOT NULL DEFAULT 'unknown', -- new / same / local_changed / remote_changed / both_changed / no_baseline_conflict / unknown / local_missing / waiting_stable / invalid
  observed_run_id TEXT, -- 逻辑关联 job_runs.id；当前观察来源；历史事实存对应 job_items/effects
  created_at INTEGER NOT NULL, -- 登记时间
  updated_at INTEGER NOT NULL, -- 核对时间
  UNIQUE (library_id,local_path_key)
);
CREATE INDEX idx_metadata_library ON metadata_files(library_id,mapping_status,comparison,id);
CREATE INDEX idx_metadata_target ON metadata_files(target_key);
CREATE INDEX idx_metadata_anchor ON metadata_files(owner_source_node_id);

-- @table metadata_decisions | 内容指纹决议；另存目标独立于共同基线持久保存。
CREATE TABLE metadata_decisions (
  id TEXT PRIMARY KEY NOT NULL, -- 决议 ID
  metadata_file_id TEXT NOT NULL, -- 逻辑关联 metadata_files.id；有决议/基线证据时禁删
  fingerprint TEXT NOT NULL UNIQUE, -- 映射+目标身份+双方可比内容摘要+基线摘要；同组合去重
  decision TEXT NOT NULL, -- keep_remote / save_copy / replace_local
  state TEXT NOT NULL, -- requested / blocked / applied / verified / obsolete
  snapshot_json TEXT NOT NULL, -- schema_version、决议时双方/目标/基线/预期版本不可变快照
  actual_target_json TEXT, -- schema_version、实际另存名称/ID/路径/内容及核验；同指纹复用
  effect_id TEXT, -- 逻辑关联 job_effects.id；已执行效果及可恢复备份
  created_at INTEGER NOT NULL, -- 决议时间
  updated_at INTEGER NOT NULL -- 状态时间
);
CREATE INDEX idx_decision_file ON metadata_decisions(metadata_file_id,created_at);
CREATE INDEX idx_decision_effect ON metadata_decisions(effect_id);

-- @table quarantine_records | 隔离事实与保留期；恢复尝试写入任务项和副作用账本。
CREATE TABLE quarantine_records (
  id TEXT PRIMARY KEY NOT NULL, -- 隔离记录 ID
  artifact_id TEXT NOT NULL, -- 逻辑关联 strm_artifacts.id；原受管产物
  quarantine_effect_id TEXT NOT NULL UNIQUE, -- 逻辑关联 job_effects.id；隔离执行及核验
  original_path TEXT NOT NULL, -- 原始实际路径
  quarantine_path TEXT NOT NULL UNIQUE, -- 扫描根之外隔离位置
  content_hash TEXT NOT NULL, -- 隔离内容 SHA-256
  size_bytes INTEGER NOT NULL, -- 已核验大小
  evidence_json TEXT NOT NULL, -- schema_version、完整扫描/范围/原文件未修改/原路径键证据
  status TEXT NOT NULL, -- pending / quarantined / restored / expired / purged / conflict
  expires_at INTEGER NOT NULL, -- 保留截止时间；不等于已经物理删除
  last_restore_item_id TEXT, -- 逻辑关联 job_items.id；最新恢复尝试；完整历史在任务项/effect
  created_at INTEGER NOT NULL, -- 登记时间
  updated_at INTEGER NOT NULL -- 实际状态时间
);
CREATE INDEX idx_quarantine_expiry ON quarantine_records(status,expires_at);
CREATE INDEX idx_quarantine_artifact ON quarantine_records(artifact_id);
