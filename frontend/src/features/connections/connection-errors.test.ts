import { describe, expect, it } from 'vitest'
import { ApiError } from '../../shared/api/client'
import { describeConnectionError } from './connection-errors'

describe('describeConnectionError', () => {
  it('gives the user a sign-in action for 401', () => {
    expect(describeConnectionError(new ApiError(401, 'session invalid')))
      .toBe('登录状态已失效，请重新登录后继续。')
  })

  it('gives the user a session refresh action for 403', () => {
    expect(describeConnectionError(new ApiError(403, 'CSRF check failed')))
      .toBe('当前会话无权执行此操作，请刷新页面后重试。')
  })
})
