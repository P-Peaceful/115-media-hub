import { request } from '../../shared/api/client'
import type { components } from '../../shared/api/generated/schema'

const base = '/api/admin/connections/115'

export type Device = components['schemas']['QRCodeDevice']
export type QRStatus = components['schemas']['QRCodeSession']['status']
export type Connection = components['schemas']['Connection']
export type ConnectionSummary = components['schemas']['ConnectionSummary']
export type QRSession = components['schemas']['QRCodeSession']
export type ConnectionTestResult = components['schemas']['ConnectionTestResult']

export const connectionKeys = {
  summary: ['connections', '115'] as const,
  qrSession: (sessionId: string) => ['connections', '115', 'qr', sessionId] as const,
}

export async function getConnectionSummary() {
  return await request<ConnectionSummary>(base) ?? { configured: false }
}

export async function createQRSession(device: Device) {
  const session = await request<QRSession>(`${base}/qr-sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device }),
  })
  if (!session) throw new Error('115 QR session response was empty')
  return session
}

export async function getQRSession(sessionId: string) {
  return await request<QRSession>(`${base}/qr-sessions/${encodeURIComponent(sessionId)}`)
}

export async function completeQRSession(sessionId: string) {
  return await request<Connection>(`${base}/qr-sessions/${encodeURIComponent(sessionId)}/complete`, { method: 'POST' })
}

export async function testConnection() {
  return await request<ConnectionTestResult>(`${base}/test`, { method: 'POST' })
}

export async function refreshAccountProfile() {
  const connection = await request<Connection>(`${base}/profile/refresh`, { method: 'POST' })
  if (!connection) throw new Error('115 profile response was empty')
  return connection
}

export function qrImagePath(sessionId: string) {
  // Only the opaque local ID enters this URL; upstream QR parameters stay on the server.
  return `${base}/qr-sessions/${encodeURIComponent(sessionId)}/qr`
}
