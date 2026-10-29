import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DirectoryEntriesPage, DirectoryEntry } from './browse-api'

const queryMocks = vi.hoisted(() => ({ useQuery: vi.fn() }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: queryMocks.useQuery,
  useQueryClient: () => ({ setQueryData: vi.fn() }),
}))

import { BrowsePage } from './BrowsePage'

const folder: DirectoryEntry = { id: 'folder-1', parentId: '0', name: '示例文件夹', isDirectory: true, size: 0, modifiedAt: null, fileType: 'folder' }
const video: DirectoryEntry = { id: 'file-1', parentId: '0', name: '示例影片.mkv', isDirectory: false, size: 1024, modifiedAt: null, fileType: 'video' }

function renderPage(entries: DirectoryEntry[] = [], isPending = false) {
  const data: DirectoryEntriesPage = { directoryId: '0', offset: 0, limit: 6, total: entries.length, entries }
  queryMocks.useQuery.mockImplementation((options?: { queryKey?: readonly string[] }) => options?.queryKey?.[1] === 'directory'
    ? { data: { directoryId: '0', name: '我的网盘', breadcrumbs: [{ directoryId: '0', name: '我的网盘' }] }, isError: false }
    : { data: isPending ? undefined : data, isPending, isError: false, refetch: vi.fn() })
  return renderToStaticMarkup(createElement(MemoryRouter, null, createElement(BrowsePage)))
}

function tableSections(markup: string) {
  return {
    header: markup.match(/<thead\b[^>]*>([\s\S]*?)<\/thead>/)?.[1] ?? '',
    body: markup.match(/<tbody\b[^>]*>([\s\S]*?)<\/tbody>/)?.[1] ?? '',
  }
}

describe('BrowsePage table semantics', () => {
  beforeEach(() => queryMocks.useQuery.mockReset())

  it('keeps names as focusable text and exposes entering only in folder actions', () => {
    const { header, body } = tableSections(renderPage([folder, video]))
    expect(header.match(/<th\b/g)).toHaveLength(6)
    expect(header).toContain('操作')
    const rows = body.match(/<tr\b[^>]*>[\s\S]*?<\/tr>/g) ?? []
    expect(rows).toHaveLength(2)
    for (const [index, entry] of [folder, video].entries()) {
      const cells = rows[index].match(/<t[dh]\b[^>]*>[\s\S]*?<\/t[dh]>/g) ?? []
      expect(cells).toHaveLength(6)
      expect(cells[0]).toContain(entry.name)
      expect(cells[0]).toMatch(/<span\b[^>]*tabindex="0"[^>]*>/)
      expect(cells[0]).not.toContain('<button')
      expect(cells[0]).not.toContain('role="button"')
      if (entry.isDirectory) {
        expect(cells[5]).toContain(`aria-label="进入 ${entry.name}"`)
        expect(cells[5]).toContain('>进入</button>')
      } else {
        expect(cells[5]).toContain('—')
        expect(cells[5]).not.toContain('<button')
      }
    }
  })

  it('preserves all six columns while showing loading placeholders', () => {
    const { header, body } = tableSections(renderPage([], true))
    expect(header.match(/<th\b/g)).toHaveLength(6)
    const rows = body.match(/<tr\b[^>]*>[\s\S]*?<\/tr>/g) ?? []
    expect(rows).toHaveLength(6)
    for (const row of rows) expect(row.match(/<t[dh]\b/g)).toHaveLength(6)
  })

  it('spans the six columns for an empty directory', () => {
    const { header, body } = tableSections(renderPage())
    expect(header.match(/<th\b/g)).toHaveLength(6)
    expect(body).toContain('colspan="6"')
    expect(body).toContain('当前目录为空')
  })
})
