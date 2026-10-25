import { describe, expect, it } from 'vitest'
import { safeReturnTo, sessionDelayUntilExpiry } from './auth-state'

describe('sessionDelayUntilExpiry', () => {
  it('returns the remaining absolute lifetime and never a negative delay', () => {
    expect(sessionDelayUntilExpiry('2026-01-01T00:00:24.000Z', Date.parse('2026-01-01T00:00:00.000Z'))).toBe(24_000)
    expect(sessionDelayUntilExpiry('2025-01-01T00:00:00.000Z', Date.parse('2026-01-01T00:00:00.000Z'))).toBe(0)
  })
})

describe('safeReturnTo', () => {
  it('keeps same-origin paths and query context', () => {
    expect(safeReturnTo('/browse?folder=abc#items')).toBe('/browse?folder=abc#items')
  })
  it('rejects external and ambiguous paths', () => {
    expect(safeReturnTo('https://evil.example')).toBe('/')
    expect(safeReturnTo('//evil.example')).toBe('/')
    expect(safeReturnTo('/\\\\evil.example')).toBe('/')
  })
})
