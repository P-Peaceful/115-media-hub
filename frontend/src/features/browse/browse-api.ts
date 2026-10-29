import { request } from '../../shared/api/client'
import type { components } from '../../shared/api/generated/schema'

const base = '/api/admin/drive/115/directories'
export type DirectoryMetadata = components['schemas']['DirectoryMetadata']
export type DirectoryEntry = components['schemas']['DirectoryEntry']
export type DirectoryEntriesPage = components['schemas']['DirectoryEntriesPage']
export type EntryType = 'all' | 'video' | 'folder' | 'image' | 'document' | 'audio' | 'archive'

export const browseKeys = {
  directory: (id: string) => ['browse', 'directory', id] as const,
  entries: (id: string, offset: number, type: EntryType, q: string) => ['browse', 'entries', id, offset, 6, type, q] as const,
}

export async function getDirectory(id: string) {
  const data = await request<DirectoryMetadata>(`${base}/${encodeURIComponent(id)}`)
  if (!data) throw new Error('目录信息响应为空')
  return data
}

export async function getDirectoryEntries(id: string, offset: number, type: EntryType, q: string) {
  const params = new URLSearchParams({ offset: String(offset), limit: '6', type })
  if (q) params.set('q', q)
  const data = await request<DirectoryEntriesPage>(`${base}/${encodeURIComponent(id)}/entries?${params}`)
  if (!data) throw new Error('目录内容响应为空')
  return data
}
