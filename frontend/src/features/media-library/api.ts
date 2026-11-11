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
      "description",
      "directory_id",
      "directory_path",
      "output_path",
    ].every((key) => typeof value[key] === "string") &&
    [
      "enabled",
      "generate_strm",
      "scrape_metadata",
      "generate_nfo",
      "preserve_structure",
    ].every((key) => typeof value[key] === "boolean") &&
    typeof value.media_type === "string" &&
    ["movie", "tv", "anime", "documentary", "mixed"].includes(
      value.media_type,
    ) &&
    (value.structure_mode === "full" || value.structure_mode === "flatten") &&
    (value.link_type === "direct" || value.link_type === "proxy") &&
    (value.link_expiry === "default" || value.link_expiry === "never") &&
    value.filename_rule === "original" &&
    value.strm_extension === ".strm" &&
    value.nfo_name === "same_as_video" &&
    typeof value.min_file_size_mb === "number" &&
    Array.isArray(value.path_mappings) &&
    value.path_mappings.every(
      (item: unknown) =>
        isRecord(item) &&
        typeof item.source === "string" &&
        typeof item.target === "string",
    )
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
