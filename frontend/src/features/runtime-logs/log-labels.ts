export const runtimeLogLevels = ["debug", "info", "warn", "error"] as const;
export type RuntimeLogLevel = (typeof runtimeLogLevels)[number];

export const runtimeLogModules = [
  "sync",
  "scan",
  "strm",
  "upload",
  "jobs",
  "library",
  "provider_115",
  "auth",
  "system",
] as const;
export type RuntimeLogModule = (typeof runtimeLogModules)[number];

export const runtimeLogRanges = ["1h", "24h", "7d", "30d", "all"] as const;
export type RuntimeLogRange = (typeof runtimeLogRanges)[number];

export const clearRuntimeLogsScopes = [
  "before_7d",
  "before_30d",
  "all",
] as const;
export type ClearRuntimeLogsScope = (typeof clearRuntimeLogsScopes)[number];

export const exportFormats = ["log", "jsonl"] as const;
export type RuntimeLogExportFormat = (typeof exportFormats)[number];

export const levelMeta: Record<
  RuntimeLogLevel,
  { label: string; color: "default" | "accent" | "warning" | "danger" }
> = {
  debug: { label: "DEBUG", color: "default" },
  info: { label: "INFO", color: "accent" },
  warn: { label: "WARN", color: "warning" },
  error: { label: "ERROR", color: "danger" },
};

export const moduleLabels: Record<RuntimeLogModule, string> = {
  sync: "同步",
  scan: "扫描",
  strm: "STRM 生成",
  upload: "回传",
  jobs: "任务调度",
  library: "媒体库",
  provider_115: "115 网盘",
  auth: "认证与安全",
  system: "系统",
};

export const rangeLabels: Record<RuntimeLogRange, string> = {
  "1h": "最近 1 小时",
  "24h": "最近 24 小时",
  "7d": "最近 7 天",
  "30d": "最近 30 天",
  all: "全部时间",
};

export const clearScopeMeta: Record<
  ClearRuntimeLogsScope,
  { label: string; description: string }
> = {
  before_7d: {
    label: "清理 7 天前",
    description: "保留最近 7 天的运行日志，删除更早的历史记录（推荐）",
  },
  before_30d: {
    label: "清理 30 天前",
    description: "保留最近 30 天的运行日志，删除超过 30 天的记录",
  },
  all: {
    label: "清空全部运行日志",
    description: "删除当前所有系统运行日志记录，此操作不可恢复",
  },
};

export function isRuntimeLogLevel(value: unknown): value is RuntimeLogLevel {
  return (
    typeof value === "string" &&
    (runtimeLogLevels as readonly string[]).includes(value)
  );
}

export function isRuntimeLogModule(value: unknown): value is RuntimeLogModule {
  return (
    typeof value === "string" &&
    (runtimeLogModules as readonly string[]).includes(value)
  );
}

export function isRuntimeLogRange(value: unknown): value is RuntimeLogRange {
  return (
    typeof value === "string" &&
    (runtimeLogRanges as readonly string[]).includes(value)
  );
}

export function isClearRuntimeLogsScope(
  value: unknown,
): value is ClearRuntimeLogsScope {
  return (
    typeof value === "string" &&
    (clearRuntimeLogsScopes as readonly string[]).includes(value)
  );
}

export function formatRuntimeLogTime(iso: string): string {
  const instant = new Date(iso);
  if (Number.isNaN(instant.getTime())) return iso;
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${instant.getFullYear()}-${pad(instant.getMonth() + 1)}-${pad(instant.getDate())} ${pad(instant.getHours())}:${pad(instant.getMinutes())}:${pad(instant.getSeconds())}`;
}

export function formatTaskBadge(jobId: string): string {
  return jobId.startsWith("#") ? jobId : `#${jobId}`;
}
