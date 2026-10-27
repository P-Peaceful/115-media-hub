import { describe, expect, it } from 'vitest'
import { qrRefetchInterval } from './qr-polling'

describe('qrRefetchInterval', () => {
  it.each(['allowed', 'expired', 'canceled', 'failed'] as const)('stops polling after %s', (status) => {
    expect(qrRefetchInterval(status)).toBe(false)
  })

  it.each(['creating', 'waiting', 'scanned'] as const)('continues polling while %s', (status) => {
    expect(qrRefetchInterval(status)).toBe(2000)
  })

  it('continues polling before the first status is available', () => {
    expect(qrRefetchInterval(undefined)).toBe(2000)
  })
})
