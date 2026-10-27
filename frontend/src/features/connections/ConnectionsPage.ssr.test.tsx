import { createElement } from 'react'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConnectionSummary } from './connections-api'

const queryMocks = vi.hoisted(() => ({ useQuery: vi.fn() }))

vi.mock('@tanstack/react-query', () => ({
  useQuery: queryMocks.useQuery,
  useQueryClient: () => ({ invalidateQueries: vi.fn(), setQueryData: vi.fn() }),
  useMutation: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false, isSuccess: false, isError: false }),
}))

vi.mock('@heroui/react', async () => {
  const React = await import('react')
  type MockProps = Record<string, unknown> & { children?: ReactNode; onPress?: () => void; isPending?: boolean }
  function Button({ children, onPress, isPending, ...props }: MockProps) {
    return React.createElement('button', { ...props, onClick: onPress, disabled: isPending }, children)
  }
  function Alert({ children, ...props }: MockProps) { return React.createElement('div', props, children) }
  Alert.Indicator = () => React.createElement('span')
  Alert.Content = ({ children }: MockProps) => React.createElement('div', null, children)
  Alert.Title = ({ children }: MockProps) => React.createElement('strong', null, children)
  Alert.Description = ({ children }: MockProps) => React.createElement('p', null, children)
  return { Button, Alert }
})

vi.mock('./connections.css', () => ({}))

import { ConnectionsPage } from './ConnectionsPage'

const profileSummary: ConnectionSummary = {
  configured: true,
  connection: {
    id: 'connection-id', name: '115 user', externalAccountId: 'uid-123', status: 'unknown',
    capabilities: [{ name: 'root_directory_read', status: 'unknown' }],
    profile: {
      accountName: '', membershipStatus: 'unknown', membershipLevel: 0,
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
    expect(markup).toContain('<fieldset')
    expect(markup).toContain('<legend')
    expect(markup).toContain('type="radio"')
    expect(markup.match(/<label/g)).toHaveLength(3)
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
      accountName: '已知账号', membershipStatus: 'active',
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
    expect(markup).toContain('VIP')
    expect(markup).toContain('1.0 GiB')
    expect(markup).toContain('2.0 TiB')
    expect(markup).toContain('<dd class="mt-1 text-sm font-medium">未知</dd>')
    expect(markup).toContain('可用')
    expect(markup).toContain('不可用')
    expect(markup).not.toContain('0 B')
  })
})
