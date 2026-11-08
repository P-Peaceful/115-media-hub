import { useQuery } from "@tanstack/react-query";

import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

type SystemStatus = components["schemas"]["SystemStatus"];

export function decodeSystemStatus(value: unknown): SystemStatus {
  if (
    typeof value !== "object" ||
    value === null ||
    !("uptime_seconds" in value) ||
    !Number.isSafeInteger(value.uptime_seconds) ||
    typeof value.uptime_seconds !== "number" ||
    value.uptime_seconds < 0 ||
    !("version" in value) ||
    typeof value.version !== "string" ||
    value.version.length === 0
  ) {
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  }
  return { uptime_seconds: value.uptime_seconds, version: value.version };
}

export function useSystemStatus() {
  return useQuery({
    queryKey: ["protected", "system", "status"],
    queryFn: async ({ signal }) =>
      decodeSystemStatus(
        await requestJson("/api/v1/system/status", { signal }),
      ),
    retry: false,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时 ${minutes} 分钟`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`;
  if (minutes > 0) return `${minutes} 分钟`;
  return `${seconds} 秒`;
}
