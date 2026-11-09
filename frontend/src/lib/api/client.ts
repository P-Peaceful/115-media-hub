import { ApiError } from "./errors";

export const sessionExpiredEvent = "mediahub:session-expired";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

const messages: Record<string, string> = {
  invalid_credentials: "用户名或密码错误，请重新输入。",
  authentication_required: "登录已过期，请重新登录。",
  csrf_invalid: "安全凭据已更新，请重试。",
  already_initialized: "系统已完成初始化，请登录。",
  setup_required: "请先完成系统初始化。",
  rate_limited: "操作过于频繁，请稍后再试。",
  validation_failed: "请检查填写的内容。",
  service_unavailable: "服务暂时不可用，请稍后重试。",
};

function decodeError(payload: unknown, status: number): ApiError {
  const error =
    isRecord(payload) && isRecord(payload.error) ? payload.error : {};
  const code = typeof error.code === "string" ? error.code : "unknown_error";
  const requestId =
    typeof error.request_id === "string" &&
    /^[A-Za-z0-9._-]{1,64}$/.test(error.request_id)
      ? error.request_id
      : undefined;
  let fields: Record<string, string> | undefined;
  if (code === "validation_failed" && isRecord(error.fields)) {
    fields = {};
    for (const [key, value] of Object.entries(error.fields)) {
      if (typeof value === "string" && value.length <= 300) fields[key] = value;
    }
  }
  return new ApiError(
    code,
    messages[code] ?? "请求未能完成，请稍后重试。",
    requestId,
    status,
    fields,
  );
}

export async function requestJson(
  path: string,
  options: RequestInit = {},
): Promise<unknown> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  let response: Response;
  try {
    response = await fetch(path, {
      ...options,
      credentials: "same-origin",
      cache: "no-store",
      headers,
    });
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new ApiError("network_error", "无法连接服务，请检查网络后重试。");
  }
  if (response.status === 204) return undefined;
  let payload: unknown;
  try {
    payload = await response.json();
  } catch (error) {
    if (options.signal?.aborted) throw error;
    if (response.status === 401 && path !== "/api/v1/auth/login") {
      window.dispatchEvent(new Event(sessionExpiredEvent));
    }
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  }
  if (!response.ok) {
    const error = decodeError(payload, response.status);
    if (response.status === 401 && error.code === "authentication_required") {
      window.dispatchEvent(new Event(sessionExpiredEvent));
    }
    throw error;
  }
  return payload;
}
