import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

export type LibraryInput = components["schemas"]["LibraryConfig"];
export type Library = components["schemas"]["Library"];
const base = "/api/v1/libraries";
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function invalid(): never {
  throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
}
function isLibrary(value: unknown): value is Library {
  if (
    !isRecord(value) ||
    typeof value.revision !== "number" ||
    !Number.isSafeInteger(value.revision) ||
    value.revision < 1
  )
    return false;

  return (
    [
      "id",
      "created_at",
      "updated_at",
      "name",
      "directory_id",
      "directory_path",
      "output_path",
      "exclude_directory_path",
      "strm_direct_domain",
    ].every((key) => typeof value[key] === "string") &&
    [
      "enabled",
      "generate_strm",
      "generate_nfo",
      "generate_poster_cover",
      "include_subdirectories",
      "follow_directory_changes",
    ].every((key) => typeof value[key] === "boolean") &&
    typeof value.media_type === "string" &&
    ["movie", "tv", "anime", "documentary", "mixed"].includes(
      value.media_type,
    ) &&
    value.language_preference === "zh-CN" &&
    (value.scrape_source === "tmdb" ||
      value.scrape_source === "metatube" ||
      value.scrape_source === "none") &&
    value.file_encoding === "UTF-8" &&
    (value.link_type === "direct" || value.link_type === "proxy") &&
    (value.strm_existing_file_policy === "skip" ||
      value.strm_existing_file_policy === "overwrite") &&
    (value.strm_link_format === "original_name" ||
      value.strm_link_format === "file_id")
  );
}
export function decodeLibrary(value: unknown): Library {
  if (!isLibrary(value)) return invalid();
  return value;
}
export async function listLibraries(signal: AbortSignal) {
  const items: Library[] = [];
  let cursor: string | null = null;
  do {
    const params = new URLSearchParams({ limit: "200" });
    if (cursor) params.set("cursor", cursor);
    const value = await requestJson(`${base}?${params}`, { signal });
    if (
      !isRecord(value) ||
      !Array.isArray(value.items) ||
      !(value.next_cursor === null || typeof value.next_cursor === "string")
    )
      return invalid();
    items.push(...value.items.map(decodeLibrary));
    cursor = value.next_cursor;
  } while (cursor);
  return items;
}
export async function getLibrary(id: string, signal: AbortSignal) {
  return decodeLibrary(
    await requestJson(`${base}/${encodeURIComponent(id)}`, { signal }),
  );
}
export async function changeLibrary(
  method: string,
  csrf: string,
  id?: string,
  body?: object,
  revision?: number,
) {
  const suffix = id ? `/${encodeURIComponent(id)}` : "";
  return requestJson(`${base}${suffix}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
      ...(revision !== undefined
        ? { "X-Library-Revision": String(revision) }
        : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
}
export async function setEnabled(csrf: string, id: string, enabled: boolean) {
  return requestJson(`${base}/${encodeURIComponent(id)}/enabled`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: JSON.stringify({ enabled }),
  });
}
