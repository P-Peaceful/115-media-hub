import { useQuery } from "@tanstack/react-query";

import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

export type FullSyncPreview = components["schemas"]["FullSyncPreview"];
export type FullSyncOutputCheck = FullSyncPreview["output"];
type FullSyncPreviewSample = FullSyncPreview["samples"][number];

const outputStatuses = [
  "ready",
  "will_create",
  "missing",
  "not_directory",
  "permission_denied",
  "unavailable",
] as const satisfies readonly FullSyncOutputCheck["status"][];

const stopReasons = [
  "scope_complete",
  "directory_limit",
  "request_limit",
  "time_limit",
  "sample_limit",
  "source_error",
] as const satisfies readonly FullSyncPreview["stop_reason"][];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isSample(value: unknown): value is FullSyncPreviewSample {
  return (
    isRecord(value) &&
    typeof value.source_path === "string" &&
    typeof value.relative_path === "string"
  );
}

function isOutputCheck(value: unknown): value is FullSyncOutputCheck {
  return (
    isRecord(value) &&
    outputStatuses.some((status) => status === value.status) &&
    typeof value.message === "string"
  );
}

function isPreview(value: unknown): value is FullSyncPreview {
  return (
    isRecord(value) &&
    value.mode === "sample" &&
    stopReasons.some((reason) => reason === value.stop_reason) &&
    value.scan_complete === (value.stop_reason === "scope_complete") &&
    isCount(value.revision) &&
    value.revision > 0 &&
    typeof value.scan_complete === "boolean" &&
    isCount(value.scanned_videos) &&
    isCount(value.candidate_count) &&
    isCount(value.failed_count) &&
    Array.isArray(value.samples) &&
    value.samples.length <= 20 &&
    value.samples.every(isSample) &&
    isOutputCheck(value.output)
  );
}

async function getPreview(libraryId: string, signal: AbortSignal) {
  const result = await requestJson(
    `/api/v1/libraries/${encodeURIComponent(libraryId)}/full-sync-preview`,
    { signal },
  );
  if (!isPreview(result)) {
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  }
  return result;
}

export function useFullSyncPreview(
  libraryId: string,
  revision: number,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["full-sync-preview", libraryId, revision],
    queryFn: ({ signal }) => getPreview(libraryId, signal),
    enabled,
    gcTime: 0,
    staleTime: "static",
    refetchInterval: false,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
}
