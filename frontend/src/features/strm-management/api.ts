import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components, operations } from "@/lib/api/generated/schema";

type Schemas = components["schemas"];
export type StrmFile = Schemas["StrmFile"];
export type StrmDetail = Schemas["StrmDetail"];
export type StrmFilters = NonNullable<
  operations["listStrmFiles"]["parameters"]["query"]
>;

const base = "/api/v1/strm";
const invalidResponse = () =>
  new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isStrmFile(value: unknown): value is StrmFile {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.library_id === "string" &&
    (value.library_name === null || typeof value.library_name === "string") &&
    typeof value.name === "string" &&
    typeof value.relative_path === "string" &&
    typeof value.source_file_id === "string" &&
    typeof value.source_path === "string" &&
    typeof value.generated_at === "string" &&
    Number.isFinite(Date.parse(value.generated_at)) &&
    isCount(value.size_bytes)
  );
}

function isStrmDetail(value: unknown): value is StrmDetail {
  return (
    isRecord(value) &&
    typeof value.output_path === "string" &&
    (value.content === null || typeof value.content === "string") &&
    (value.file_state === "present" ||
      value.file_state === "missing" ||
      value.file_state === "modified" ||
      value.file_state === "unavailable") &&
    isStrmFile(value)
  );
}

export async function listStrm(filters: StrmFilters, signal: AbortSignal) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) params.set(key, String(value));
  }
  const data = await requestJson(`${base}?${params}`, { signal });
  if (
    !isRecord(data) ||
    !Array.isArray(data.items) ||
    !data.items.every(isStrmFile) ||
    !isCount(data.total) ||
    !isCount(data.page) ||
    data.page < 1 ||
    !isCount(data.limit) ||
    data.limit < 1
  ) {
    throw invalidResponse();
  }
  return {
    items: data.items,
    total: data.total,
    page: data.page,
    limit: data.limit,
  } satisfies Schemas["StrmPage"];
}

export async function getStrm(id: string, signal: AbortSignal) {
  const data = await requestJson(`${base}/${encodeURIComponent(id)}`, {
    signal,
  });
  if (!isStrmDetail(data)) throw invalidResponse();
  return data;
}
