import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConnectionSummary } from './connections-api'

const queryMocks = vi.hoisted(() => ({ useQuery: vi.fn() }))

vi.mock('@tanstack/react-query', () => ({
  useQuery: queryMocks.useQuery,
  useQueryClient: () => ({ invalidateQueries: vi.fn(), setQueryData: vi.fn() }),
  useMutation: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false, isSuccess: false, isError: false }),
}))

vi.mock('./connections.css', () => ({}))

import { ConnectionsPage } from './ConnectionsPage'
import { ConnectionSidebarCard } from './ConnectionSidebarCard'
import { MemoryRouter } from 'react-router'

const profileSummary: ConnectionSummary = {
  configured: true,
  connection: {
    id: 'connection-id', name: '115 user', externalAccountId: 'uid-123', status: 'unknown',
    capabilities: [{ name: 'root_directory_read', status: 'unknown' }],
    profile: {
      accountName: '', membershipStatus: 'unknown',
      storageTotalBytes: null, storageUsedBytes: null, storageFreeBytes: null, deviceCount: null,
    },
    updatedAt: '2026-10-27T00:00:00Z',
  },
}

function renderPage(summary: { isLoading: boolean; isError: boolean; data?: ConnectionSummary }) {
  queryMocks.useQuery.mockImplementation((options?: { queryKey?: unknown[] }) =>
    options?.queryKey?.[2] === 'qr'
      ? { data: undefined }
      : { ...summary, refetch: vi.fn() },
  )
  return renderToStaticMarkup(createElement(ConnectionsPage))
}

describe('ConnectionsPage server-rendered semantics', () => {
  beforeEach(() => queryMocks.useQuery.mockReset())

  it('announces loading state to assistive technology', () => {
    const markup = renderPage({ isLoading: true, isError: false })
    expect(markup).toContain('role="status"')
    expect(markup).toContain('正在读取连接状态')
  })

  it('associates each device radio with a label and exposes Web guidance', () => {
    const markup = renderPage({ isLoading: false, isError: false, data: { configured: false } })
    expect(markup).toContain('role="radiogroup"')
    expect(markup).toContain('设备方式')
    expect(markup).toContain('type="radio"')
    const deviceLabels = markup.match(/<label\b[^>]*>[\s\S]*?<\/label>/g) ?? []
    expect(deviceLabels).toHaveLength(3)
    for (const deviceLabel of ['Web 网页', '支付宝小程序', '微信小程序']) {
      expect(deviceLabels.some((label) => label.includes('type="radio"') && label.includes(deviceLabel))).toBe(true)
    }
    expect(markup).toContain('Web 登录提示')
    expect(markup).toContain('role="note"')
  })

  it('renders unknown account and capability values instead of fabricated zero values', () => {
    const summaryWithUnexpectedCredential = structuredClone(profileSummary)
    Object.assign(summaryWithUnexpectedCredential.connection!, { credentialJson: 'CREDENTIAL_SENTINEL' })
    const markup = renderPage({ isLoading: false, isError: false, data: summaryWithUnexpectedCredential })
    expect(markup).toContain('会员信息未知')
    expect(markup).toContain('未知')
    expect(markup).toContain('读取网盘根目录')
    expect(markup).toContain('aria-labelledby="current-connection-title"')
    expect(markup).not.toContain('CREDENTIAL_SENTINEL')
  })

  it('renders known partial profile and capability evidence while preserving missing values as unknown', () => {
    const partialSummary = structuredClone(profileSummary)
    Object.assign(partialSummary.connection!.profile, {
      accountName: '已知账号', membershipStatus: 'active', membershipName: '上游会员名',
      storageTotalBytes: 2 * 1024 ** 4,
      storageUsedBytes: 1024 ** 3,
      storageFreeBytes: null,
    })
    partialSummary.connection!.capabilities = [
      { name: 'account_identity', status: 'available' },
      { name: 'root_directory_read', status: 'unavailable' },
    ]
    const markup = renderPage({ isLoading: false, isError: false, data: partialSummary })

    expect(markup).toContain('已知账号')
    expect(markup).toContain('上游会员名')
    expect(markup).toContain('1.0 GiB')
    expect(markup).toContain('2.0 TiB')
    expect(markup).toContain('<dd class="mt-1 text-sm font-medium">未知</dd>')
    expect(markup).toContain('可用')
    expect(markup).toContain('不可用')
    expect(markup).not.toContain('0 B')
  })
})

it('shows the upstream name verbatim and escaped on page and sidebar', () => {
  const summary = structuredClone(profileSummary)
  Object.assign(summary.connection!.profile, { membershipName: '未来 & <尊享会员>', membershipStatus: 'unknown' })
  const page = renderPage({ isLoading: false, isError: false, data: summary })
  expect(page).toContain('未来 &amp; &lt;尊享会员&gt;')
  const sidebar = renderToStaticMarkup(createElement(MemoryRouter, null, createElement(ConnectionSidebarCard)))
  expect(sidebar).toContain('>未来 &amp; &lt;尊享会员&gt;</span>')
  expect(page + sidebar).not.toContain('<尊享会员>')
})

it('shows unknown for old snapshots even with an active numeric membership', () => {
  const summary = structuredClone(profileSummary)
  Object.assign(summary.connection!.profile, { membershipStatus: 'active', membershipLevel: 8 })
  const page = renderPage({ isLoading: false, isError: false, data: summary })
  const sidebar = renderToStaticMarkup(createElement(MemoryRouter, null, createElement(ConnectionSidebarCard)))
  expect(page).toContain('会员信息未知')
  expect(sidebar).toContain('>会员信息未知</span>')
  expect(page + sidebar).not.toContain('VIP')
})
