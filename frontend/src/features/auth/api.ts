import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

export type AuthBootstrap = components["schemas"]["AuthBootstrap"];
export type AuthSession = components["schemas"]["AuthSession"];
export type LoginRequest = components["schemas"]["LoginRequest"];
export type InitializeRequest = components["schemas"]["InitializeRequest"];

function isAdministrator(
  value: unknown,
): value is components["schemas"]["Administrator"] {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    value.id.length > 0 &&
    "username" in value &&
    typeof value.username === "string"
  );
}

function isExpiry(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
  );
}

function isToken(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 512;
}

export function decodeSession(value: unknown): AuthSession {
  if (
    typeof value !== "object" ||
    value === null ||
    !("administrator" in value) ||
    !isAdministrator(value.administrator) ||
    !("expires_at" in value) ||
    !isExpiry(value.expires_at) ||
    !("csrf_token" in value) ||
    !isToken(value.csrf_token)
  )
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  return {
    administrator: value.administrator,
    expires_at: value.expires_at,
    csrf_token: value.csrf_token,
  };
}

export function decodeBootstrap(value: unknown): AuthBootstrap {
  if (
    typeof value !== "object" ||
    value === null ||
    !("initialized" in value) ||
    typeof value.initialized !== "boolean" ||
    !("authenticated" in value) ||
    typeof value.authenticated !== "boolean" ||
    !("csrf_token" in value) ||
    !isToken(value.csrf_token) ||
    !("administrator" in value) ||
    !("expires_at" in value)
  )
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  if (value.authenticated) {
    if (!value.initialized)
      throw new ApiError(
        "invalid_response",
        "服务返回了无效响应，请稍后重试。",
      );
    return { initialized: true, authenticated: true, ...decodeSession(value) };
  }
  if (value.administrator !== null || value.expires_at !== null) {
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  }
  return {
    initialized: value.initialized,
    authenticated: false,
    administrator: null,
    expires_at: null,
    csrf_token: value.csrf_token,
  };
}

export async function getBootstrap(signal: AbortSignal) {
  return decodeBootstrap(
    await requestJson("/api/v1/auth/bootstrap", { signal }),
  );
}

async function startSession(
  action: "login" | "initialize",
  input: LoginRequest | InitializeRequest,
  csrfToken: string,
) {
  return decodeSession(
    await requestJson(`/api/v1/auth/${action}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrfToken,
      },
      body: JSON.stringify(input),
    }),
  );
}

export function login(input: LoginRequest, csrfToken: string) {
  return startSession("login", input, csrfToken);
}

export function initialize(input: InitializeRequest, csrfToken: string) {
  return startSession("initialize", input, csrfToken);
}

export async function logout(csrfToken: string) {
  await requestJson("/api/v1/auth/logout", {
    method: "POST",
    headers: { "X-CSRF-Token": csrfToken },
  });
}
