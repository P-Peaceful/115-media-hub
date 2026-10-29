import { describe, expect, it } from 'vitest'
import { formatMembershipName } from './profile-format'

describe('upstream membership name', () => {
  it('preserves unfamiliar text including whitespace', () => {
    expect(formatMembershipName('  未知新会员名 & <未来>  ')).toBe('  未知新会员名 & <未来>  ')
  })
  it('keeps missing and blank names unknown', () => {
    for (const name of [null, undefined, '', ' \t ']) expect(formatMembershipName(name)).toBe('会员信息未知')
  })
})
