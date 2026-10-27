import { useQuery } from '@tanstack/react-query'
import { Link2 } from 'lucide-react'
import { useNavigate } from 'react-router'
import { getConnectionSummary, connectionKeys } from './connections-api'
import { formatBytes } from './profile-format'

export function ConnectionSidebarCard() {
  const navigate = useNavigate()
  const query = useQuery({
    queryKey: connectionKeys.summary,
    queryFn: getConnectionSummary,
    staleTime: 30_000,
  })
  const connection = query.data?.connection
  const profile = connection?.profile
  const status = query.isError
    ? '无法读取授权信息'
    : query.isLoading
      ? '正在读取授权信息…'
      : !connection
        ? '点击此处授权 115 网盘'
        : `UID ${connection.externalAccountId} · ${profile?.membershipStatus === 'active' ? 'VIP' : profile?.membershipStatus === 'ordinary' ? '普通用户' : '会员信息未知'} · ${formatBytes(profile?.storageUsedBytes)} / ${formatBytes(profile?.storageTotalBytes)}`

  return (
    <button
      type="button"
      onClick={() => navigate('/connections')}
      aria-label={connection ? `115 网盘，${connection.name}，${status}，打开连接管理` : `115 网盘，${status}，打开连接管理`}
      className="mt-4 flex w-full items-center gap-2.5 rounded-xl border border-separator bg-surface-secondary p-3 text-left transition-colors hover:bg-default max-[720px]:size-11 max-[720px]:justify-center max-[720px]:p-0"
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-soft-foreground"><Link2 size={16} /></span>
      <span className="min-w-0 max-[720px]:hidden">
        <span className="block truncate text-xs font-semibold text-foreground">{connection ? profile?.accountName || connection.name : '115 网盘授权'}</span>
        <span className="mt-0.5 block truncate text-[11px] text-muted">{status}</span>
      </span>
    </button>
  )
}
