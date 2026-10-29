import { Button, Chip } from '@heroui/react'
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
    <Button
      type="button"
      fullWidth
      variant="secondary"
      className="h-auto min-h-[72px] flex-col items-start gap-2 rounded-xl bg-background px-3 py-3 font-normal max-[720px]:min-h-10 max-[720px]:items-center max-[720px]:px-0"
      onPress={() => navigate('/connections')}
      aria-label={connection ? `115 网盘，${connection.name}，${status}，打开连接管理` : `115 网盘，${status}，打开连接管理`}
    >
      <Link2 aria-hidden size={18} className="hidden max-[720px]:block" />
      <span className="w-full min-w-0 truncate text-left text-sm max-[720px]:hidden">
        {connection ? `115 · ${profile?.accountName || connection.name}` : '115 网盘授权'}
      </span>
      <Chip size="sm" color={query.isError ? 'danger' : connection ? 'success' : 'default'} variant="soft" className="max-w-full max-[720px]:hidden">
        <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-current" />
        {query.isError ? '授权信息不可用' : query.isLoading ? '正在读取…' : connection ? '已授权' : '尚未连接'}
      </Chip>
    </Button>
  )
}
