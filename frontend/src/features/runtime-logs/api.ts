import { requestJson, sessionExpiredEvent } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";

import type {
  ClearRuntimeLogsScope,
  RuntimeLogExportFormat,
  RuntimeLogLevel,
  RuntimeLogModule,
  RuntimeLogRange,
} from "./log-labels";
import {
  isClearRuntimeLogsScope,
  isRuntimeLogLevel,
  isRuntimeLogModule,
} from "./log-labels";

export type RuntimeLogDetailValue = string | number | boolean;

export type RuntimeLogEntry = {
  id: string;
  occurred_at: string;
  level: RuntimeLogLevel;
  module: RuntimeLogModule;
  operation: string;
  job_id: string | null;
  request_id: string | null;
  error_code: string | null;
  message: string;
  details: Record<string, RuntimeLogDetailValue>;
};

export type RuntimeLogPage = {
  items: RuntimeLogEntry[];
  next_cursor: string | null;
};

export type RuntimeLogFilters = {
  range: RuntimeLogRange;
  level?: RuntimeLogLevel;
  module?: RuntimeLogModule;
  job_id?: string;
  q?: string;
  limit?: number;
  cursor?: string;
};

export type ClearRuntimeLogsResult = {
  deleted_count: number;
  scope: ClearRuntimeLogsScope;
};

export type ExportRuntimeLogsResult = {
  filename: string;
  truncated: boolean;
};

const base = "/api/v1/logs";

const invalidResponse = () =>
  new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string";
}

function isDetailsRecord(
  value: unknown,
): value is Record<string, RuntimeLogDetailValue> {
  if (!isRecord(value)) return false;
  for (const item of Object.values(value)) {
    if (
      typeof item !== "string" &&
      typeof item !== "number" &&
      typeof item !== "boolean"
    ) {
      return false;
    }
  }
  return true;
}

export function isRuntimeLogEntry(value: unknown): value is RuntimeLogEntry {
  if (!isRecord(value)) return false;
  if (typeof value.id !== "string" || value.id.length === 0) return false;
  if (
    typeof value.occurred_at !== "string" ||
    !Number.isFinite(Date.parse(value.occurred_at))
  ) {
    return false;
  }
  if (!isRuntimeLogLevel(value.level)) return false;
  if (!isRuntimeLogModule(value.module)) return false;
  if (typeof value.operation !== "string") return false;
  if (!isNullableString(value.job_id)) return false;
  if (!isNullableString(value.request_id)) return false;
  if (!isNullableString(value.error_code)) return false;
  if (typeof value.message !== "string") return false;
  if (value.details !== undefined && !isDetailsRecord(value.details)) {
    return false;
  }
  return true;
}

function normalizeEntry(entry: RuntimeLogEntry): RuntimeLogEntry {
  return {
    ...entry,
    details: entry.details ?? {},
  };
}

export function buildRuntimeLogQueryParams(
  filters: RuntimeLogFilters,
  options: { includePagination?: boolean; includeRange?: boolean } = {},
): URLSearchParams {
  const { includePagination = true, includeRange = true } = options;
  const params = new URLSearchParams();
  if (includeRange) params.set("range", filters.range);
  if (filters.level) params.set("level", filters.level);
  if (filters.module) params.set("module", filters.module);
  if (filters.job_id) params.set("job_id", filters.job_id);
  if (filters.q) params.set("q", filters.q);
  if (includePagination) {
    if (filters.limit !== undefined) params.set("limit", String(filters.limit));
    if (filters.cursor) params.set("cursor", filters.cursor);
  }
  return params;
}

export async function listRuntimeLogs(
  filters: RuntimeLogFilters,
  signal: AbortSignal,
): Promise<RuntimeLogPage> {
  const params = buildRuntimeLogQueryParams(filters);
  const data = await requestJson(`${base}?${params}`, { signal });
  if (
    !isRecord(data) ||
    !Array.isArray(data.items) ||
    !data.items.every(isRuntimeLogEntry) ||
    !isNullableString(data.next_cursor)
  ) {
    throw invalidResponse();
  }
  return {
    items: data.items.map(normalizeEntry),
    next_cursor: data.next_cursor,
  };
}

export async function clearRuntimeLogs(
  csrfToken: string,
  scope: ClearRuntimeLogsScope,
): Promise<ClearRuntimeLogsResult> {
  const data = await requestJson(`${base}/clear`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken,
    },
    body: JSON.stringify({ scope }),
  });
  if (
    !isRecord(data) ||
    typeof data.deleted_count !== "number" ||
    !Number.isSafeInteger(data.deleted_count) ||
    data.deleted_count < 0 ||
    !isClearRuntimeLogsScope(data.scope)
  ) {
    throw invalidResponse();
  }
  return {
    deleted_count: data.deleted_count,
    scope: data.scope,
  };
}

function parseFilename(
  disposition: string | null,
  format: RuntimeLogExportFormat,
): string {
  if (disposition) {
    const utf8Match = /filename\*=UTF-8''([^;\s]+)/i.exec(disposition);
    if (utf8Match?.[1]) {
      try {
        return decodeURIComponent(utf8Match[1]);
      } catch {
        // Fall through to plain filename parsing.
      }
    }
    const quotedMatch = /filename="([^"]+)"/i.exec(disposition);
    if (quotedMatch?.[1]) return quotedMatch[1];
    const plainMatch = /filename=([^;\s]+)/i.exec(disposition);
    if (plainMatch?.[1]) return plainMatch[1];
  }
  return `mediahub-logs.${format}`;
}

const exportErrorMessages: Record<string, string> = {
  authentication_required: "登录已过期，请重新登录。",
  validation_failed: "筛选参数无效，请重置后重试。",
  service_unavailable: "日志导出暂时不可用，请稍后重试。",
};

async function decodeBinaryError(response: Response): Promise<ApiError> {
  let code = "unknown_error";
  let requestId: string | undefined;
  try {
    const payload: unknown = await response.json();
    if (isRecord(payload) && isRecord(payload.error)) {
      if (typeof payload.error.code === "string") code = payload.error.code;
      if (
        typeof payload.error.request_id === "string" &&
        /^[A-Za-z0-9._-]{1,64}$/.test(payload.error.request_id)
      ) {
        requestId = payload.error.request_id;
      }
    }
  } catch {
    // Non-JSON error body; keep fallback error code.
  }
  if (response.status === 401) {
    window.dispatchEvent(new Event(sessionExpiredEvent));
    return new ApiError(
      "authentication_required",
      exportErrorMessages.authentication_required ?? "登录已过期，请重新登录。",
      requestId,
      response.status,
    );
  }
  return new ApiError(
    code,
    exportErrorMessages[code] ?? "下载日志失败，请稍后重试。",
    requestId,
    response.status,
  );
}

export async function exportRuntimeLogs(
  filters: RuntimeLogFilters,
  format: RuntimeLogExportFormat,
): Promise<ExportRuntimeLogsResult> {
  const params = buildRuntimeLogQueryParams(filters, {
    includePagination: false,
  });
  params.set("format", format);
  let response: Response;
  try {
    response = await fetch(`${base}/export?${params}`, {
      method: "GET",
      credentials: "same-origin",
      cache: "no-store",
    });
  } catch {
    throw new ApiError("network_error", "无法连接服务，请检查网络后重试。");
  }
  if (!response.ok) {
    throw await decodeBinaryError(response);
  }
  const blob = await response.blob();
  const filename = parseFilename(
    response.headers.get("Content-Disposition"),
    format,
  );
  const truncated =
    response.headers.get("X-Export-Truncated")?.toLowerCase() === "true";
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
  return { filename, truncated };
}

export function decodeRuntimeLogCreatedEvent(
  event: Event,
): RuntimeLogEntry | null {
  if (!(event instanceof MessageEvent) || typeof event.data !== "string") {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(event.data);
    if (!isRuntimeLogEntry(parsed)) return null;
    return normalizeEntry(parsed);
  } catch {
    return null;
  }
}

export function decodeRuntimeLogClearedEvent(event: Event): boolean {
  if (!(event instanceof MessageEvent)) return false;
  if (typeof event.data !== "string" || event.data.trim() === "") return true;
  try {
    const parsed: unknown = JSON.parse(event.data);
    return isRecord(parsed);
  } catch {
    return false;
  }
}

export function buildRuntimeLogStreamUrl(filters: RuntimeLogFilters): string {
  const params = buildRuntimeLogQueryParams(filters, {
    includePagination: false,
    includeRange: false,
  });
  const query = params.toString();
  return query ? `${base}/stream?${query}` : `${base}/stream`;
}
