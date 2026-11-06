import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

type HealthResponse = components["schemas"]["HealthResponse"];

function isHealthResponse(value: unknown): value is HealthResponse {
  return (
    typeof value === "object" &&
    value !== null &&
    "status" in value &&
    value.status === "ok"
  );
}

function getRequestId(value: unknown): string | undefined {
  if (typeof value !== "object" || value === null || !("error" in value))
    return;
  const error = value.error;
  if (typeof error !== "object" || error === null || !("request_id" in error))
    return;
  if (
    typeof error.request_id === "string" &&
    /^[A-Za-z0-9._-]{1,64}$/.test(error.request_id)
  )
    return error.request_id;
}

export async function getReadiness(
  signal: AbortSignal,
): Promise<HealthResponse> {
  let response: Response;
  try {
    response = await fetch("/health/ready", {
      signal,
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
  } catch (error) {
    if (signal.aborted) throw error;
    throw new ApiError("network_error", "无法连接服务，请检查网络后重试。");
  }
  let payload: unknown;
  try {
    payload = await response.json();
  } catch (error) {
    if (signal.aborted) throw error;
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  }
  if (!response.ok) {
    throw new ApiError(
      "service_unavailable",
      "服务暂未就绪，请稍后重试。",
      getRequestId(payload),
    );
  }
  if (!isHealthResponse(payload))
    throw new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");
  return payload;
}
