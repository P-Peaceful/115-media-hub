import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

export type DirectoryListing = components["schemas"]["CloudDrivePage"];
export type DriveItem = DirectoryListing["items"][number];

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function invalid(): never {
  throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
}
function nullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string";
}
export function decodeListing(value: unknown): DirectoryListing {
  if (
    !record(value) ||
    typeof value.directory_id !== "string" ||
    !Array.isArray(value.path) ||
    !Array.isArray(value.items) ||
    typeof value.total !== "number" ||
    !Number.isSafeInteger(value.total) ||
    value.total < 0 ||
    !(
      value.page === null ||
      (typeof value.page === "number" &&
        Number.isSafeInteger(value.page) &&
        value.page >= 1)
    ) ||
    typeof value.page_size !== "number" ||
    !Number.isInteger(value.page_size) ||
    value.page_size < 1 ||
    value.page_size > 200 ||
    value.items.length > value.page_size ||
    (typeof value.page === "number" &&
      value.page - 1 > Math.floor(Number.MAX_SAFE_INTEGER / value.page_size)) ||
    typeof value.total_pages !== "number" ||
    !Number.isSafeInteger(value.total_pages) ||
    value.total_pages !==
      Math.max(1, Math.ceil(value.total / value.page_size)) ||
    !nullableString(value.next_cursor)
  )
    return invalid();
  const path = value.path.map((part: unknown) => {
    if (
      !record(part) ||
      typeof part.id !== "string" ||
      typeof part.name !== "string"
    )
      return invalid();
    return { id: part.id, name: part.name };
  });
  const items = value.items.map((item: unknown) => {
    if (
      !record(item) ||
      typeof item.id !== "string" ||
      typeof item.name !== "string" ||
      typeof item.is_directory !== "boolean" ||
      !nullableString(item.size_bytes) ||
      (typeof item.size_bytes === "string" && !/^\d+$/.test(item.size_bytes)) ||
      !nullableString(item.modified_at) ||
      typeof item.file_type !== "string"
    )
      return invalid();
    return {
      id: item.id,
      name: item.name,
      is_directory: item.is_directory,
      size_bytes: item.size_bytes,
      modified_at: item.modified_at,
      file_type: item.file_type,
    };
  });
  return {
    directory_id: value.directory_id,
    total: value.total,
    page: value.page,
    page_size: value.page_size,
    total_pages: value.total_pages,
    path,
    items,
    next_cursor: value.next_cursor,
  };
}
export async function getDirectory(
  params: URLSearchParams,
  signal: AbortSignal,
) {
  const listing = decodeListing(
    await requestJson(`/api/v1/cloud-drive/files?${params}`, { signal }),
  );
  if (params.has("page") && listing.page !== Number(params.get("page")))
    return invalid();
  return listing;
}
