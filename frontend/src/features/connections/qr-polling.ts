import type { QRStatus } from './connections-api'

const terminalStatuses: ReadonlySet<QRStatus> = new Set(['allowed', 'expired', 'canceled', 'failed'])

/** Returns false for completed QR sessions so React Query stops polling. */
export function qrRefetchInterval(status: QRStatus | undefined): number | false {
  return status && terminalStatuses.has(status) ? false : 2000
}
