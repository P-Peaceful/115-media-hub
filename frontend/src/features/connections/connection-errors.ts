import { ApiError } from '../../shared/api/client'

/** Maps safe API errors to actionable text without exposing arbitrary server data. */
export function describeConnectionError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return '登录状态已失效，请重新登录后继续。'
    if (error.status === 403) return '当前会话无权执行此操作，请刷新页面后重试。'
    if (error.status === 409) return '授权账号与现有连接不一致，原连接保持不变。'
    if (error.status === 404) return '扫码会话已失效，请重新发起授权。'
    if (error.upstreamCode) return `${error.message}（115 错误码 ${error.upstreamCode}）`
    if (error.status === 503) return error.message
  }
  return '请求未能完成，请检查服务状态后重试。'
}
