import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

type Security = components["schemas"]["SecuritySettings"];
type Account = components["schemas"]["Provider115Account"];
type Qr = components["schemas"]["Provider115QR"];
type Credentials = components["schemas"]["SecurityCredentialsRequest"];

function invalid(): never {
  throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
}
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
export function decodeSecurity(value: unknown): Security {
  if (
    !record(value) ||
    typeof value.username !== "string" ||
    (value.session_lifetime_days !== 1 &&
      value.session_lifetime_days !== 7 &&
      value.session_lifetime_days !== 30)
  )
    return invalid();
  return {
    username: value.username,
    session_lifetime_days: value.session_lifetime_days,
  };
}
export function decodeAccount(value: unknown): Account {
  if (
    !record(value) ||
    typeof value.connected !== "boolean" ||
    !(value.account_name === null || typeof value.account_name === "string") ||
    !(value.connected_at === null || typeof value.connected_at === "string")
  )
    return invalid();
  return {
    connected: value.connected,
    account_name: value.account_name,
    connected_at: value.connected_at,
  };
}
export function decodeQr(value: unknown): Qr {
  if (
    !record(value) ||
    typeof value.id !== "string" ||
    typeof value.qr_url !== "string" ||
    !value.qr_url.startsWith("data:image/png;base64,") ||
    typeof value.expires_at !== "string" ||
    (value.status !== "waiting" &&
      value.status !== "scanned" &&
      value.status !== "succeeded" &&
      value.status !== "expired" &&
      value.status !== "canceled")
  )
    return invalid();
  return {
    id: value.id,
    qr_url: value.qr_url,
    expires_at: value.expires_at,
    status: value.status,
  };
}
const base = "/api/v1/settings";
function mutation(method: string, csrf: string, body?: object): RequestInit {
  return {
    method,
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    ...(body ? { body: JSON.stringify(body) } : {}),
  };
}
export async function getSecurity(signal: AbortSignal) {
  return decodeSecurity(await requestJson(`${base}/security`, { signal }));
}
export async function updateCredentials(csrf: string, body: Credentials) {
  return decodeSecurity(
    await requestJson(
      `${base}/security/credentials`,
      mutation("PUT", csrf, body),
    ),
  );
}
export async function updateSession(
  csrf: string,
  days: Security["session_lifetime_days"],
) {
  return decodeSecurity(
    await requestJson(
      `${base}/security/session`,
      mutation("PUT", csrf, { session_lifetime_days: days }),
    ),
  );
}
export async function getAccount(signal: AbortSignal) {
  return decodeAccount(await requestJson(`${base}/115`, { signal }));
}
export async function createQr(csrf: string) {
  return decodeQr(await requestJson(`${base}/115/qr`, mutation("POST", csrf)));
}
export async function getQr(id: string, csrf: string, signal: AbortSignal) {
  return decodeQr(
    await requestJson(`${base}/115/qr/${encodeURIComponent(id)}/poll`, {
      ...mutation("POST", csrf),
      signal,
    }),
  );
}
export async function cancelQr(csrf: string, id: string) {
  await requestJson(
    `${base}/115/qr/${encodeURIComponent(id)}`,
    mutation("DELETE", csrf),
  );
}
export async function removeAccount(csrf: string) {
  await requestJson(`${base}/115`, mutation("DELETE", csrf));
}
