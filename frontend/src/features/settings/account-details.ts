import { useQuery } from "@tanstack/react-query";

import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

type Details = components["schemas"]["Provider115Details"];
export const accountDetailsKey = ["settings", "115", "details"] as const;

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function nullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string";
}
function isProfile(
  value: unknown,
): value is components["schemas"]["Provider115Profile"] {
  return (
    record(value) &&
    typeof value.user_name === "string" &&
    nullableString(value.user_id) &&
    nullableString(value.avatar) &&
    (value.is_member === null || typeof value.is_member === "boolean") &&
    (value.member_expires_at === null ||
      (typeof value.member_expires_at === "string" &&
        !Number.isNaN(Date.parse(value.member_expires_at)))) &&
    (value.member_forever === null || typeof value.member_forever === "boolean")
  );
}
function isDevice(value: unknown): value is Details["devices"][number] {
  return (
    record(value) &&
    typeof value.name === "string" &&
    typeof value.type === "string" &&
    typeof value.ip === "string" &&
    typeof value.city === "string" &&
    (value.login_at === null ||
      (typeof value.login_at === "string" &&
        !Number.isNaN(Date.parse(value.login_at)))) &&
    typeof value.is_current === "boolean"
  );
}
function isSpace(
  value: unknown,
): value is components["schemas"]["Provider115Space"] {
  return (
    record(value) &&
    typeof value.used_bytes === "string" &&
    typeof value.total_bytes === "string" &&
    /^\d+$/.test(value.used_bytes) &&
    /^\d+$/.test(value.total_bytes)
  );
}
export function decodeAccountDetails(value: unknown): Details {
  if (
    !record(value) ||
    (value.status !== "valid" &&
      value.status !== "invalid" &&
      value.status !== "unconfigured") ||
    (value.space_status !== "available" &&
      value.space_status !== "unavailable") ||
    (value.devices_status !== "available" &&
      value.devices_status !== "unavailable") ||
    !Array.isArray(value.devices) ||
    !value.devices.every(isDevice) ||
    !(value.profile === null || isProfile(value.profile)) ||
    !(value.space === null || isSpace(value.space)) ||
    (value.status === "valid" && value.profile === null) ||
    (value.status !== "valid" &&
      (value.profile !== null || value.space !== null)) ||
    (value.space_status === "available") !== (value.space !== null)
  )
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  return {
    status: value.status,
    profile: value.profile,
    space: value.space,
    space_status: value.space_status,
    devices: value.devices,
    devices_status: value.devices_status,
  };
}
export function useAccountDetails() {
  return useQuery({
    queryKey: accountDetailsKey,
    queryFn: async ({ signal }) =>
      decodeAccountDetails(
        await requestJson("/api/v1/settings/115/details", { signal }),
      ),
    staleTime: 30_000,
    retry: false,
  });
}
export function formatBytes(value: string | null | undefined) {
  if (value == null) return "暂无数据";
  const bytes = BigInt(value);
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  let divisor = 1n;
  let unit = 0;
  while (bytes >= divisor * 1024n && unit < units.length - 1) {
    divisor *= 1024n;
    unit++;
  }
  const hundredths = (bytes * 100n) / divisor;
  return `${hundredths / 100n}${unit ? `.${String(hundredths % 100n).padStart(2, "0")}` : ""} ${units[unit]}`;
}

export function getCapacityMetrics(
  space: Details["space"],
): { remaining: string; percent: number; percentLabel: string } | null {
  if (!space) return null;
  const used = BigInt(space.used_bytes);
  const total = BigInt(space.total_bytes);
  if (total <= 0n) return null;
  const remaining = total > used ? total - used : 0n;
  const percent = Number((used * 1000n) / total) / 10;
  return {
    remaining: remaining.toString(),
    percent: Math.min(100, percent),
    percentLabel: `${Math.min(100, percent).toFixed(1)}%`,
  };
}

export function maskAccountId(userId: string | null | undefined) {
  if (!userId) return "暂无账号 ID";
  if (userId.length <= 7) return `****${userId.slice(-4)}`;
  return `${userId.slice(0, 3)}****${userId.slice(-4)}`;
}

export function formatCheckedAt(timestamp: number | undefined) {
  if (!timestamp) return "尚未检测";
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(timestamp);
}
