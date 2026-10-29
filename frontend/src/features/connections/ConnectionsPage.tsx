import { Alert, Avatar, Button, Card, Chip, Label, ProgressBar, Radio, RadioGroup } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import {
  completeQRSession, connectionKeys, createQRSession, getConnectionSummary, getQRSession,
  qrImagePath, refreshAccountProfile, testConnection, type ConnectionSummary, type Device, type QRStatus,
} from './connections-api'
import { formatBytes, formatMembershipName } from './profile-format'
import { qrRefetchInterval } from './qr-polling'
import { describeConnectionError } from './connection-errors'
import './connections.css'

const deviceOptions: { value: Device; label: string }[] = [
  { value: 'web', label: 'Web 网页' },
  { value: 'alipaymini', label: '支付宝小程序' },
  { value: 'wechatmini', label: '微信小程序' },
]

const qrStatusLabels: Record<QRStatus, string> = {
  creating: '正在准备二维码', waiting: '等待扫码', scanned: '已扫码，请在 115 中确认',
  allowed: '已确认，正在验证连接', expired: '二维码已过期', canceled: '扫码已取消', failed: '扫码失败',
}

function safeAvatarUrl(value: string | null | undefined) {
  if (!value) return undefined
  try {
    const parsed = new URL(value)
    const host = parsed.hostname.toLowerCase()
    const allowedHost = host === '115.com' || host.endsWith('.115.com') || host === '115cdn.com' || host.endsWith('.115cdn.com')
    return parsed.protocol === 'https:' && allowedHost ? parsed.href : undefined
  } catch { return undefined }
}

function storagePercent(used?: number | null, total?: number | null) {
  if (used == null || total == null || total <= 0) return undefined
  return Math.max(0, Math.min(100, used / total * 100))
}

function capabilityName(name: string) {
  if (name === 'account_identity') return '账号身份'
  if (name === 'root_directory_read') return '读取网盘根目录'
  return name
}

function capabilityStatus(status: string) {
  if (status === 'available') return '可用'
  if (status === 'unavailable') return '不可用'
  return '未知'
}

function connectionStatus(status: string) {
  const labels: Record<string, string> = {
    unchecked: '尚未检查', available: '可用', invalid: '凭据失效', limited: '能力受限', unreachable: '暂时无法连接', unknown: '状态未知',
  }
  return labels[status] ?? '状态未知'
}

export function ConnectionsPage() {
  const queryClient = useQueryClient()
  const [device, setDevice] = useState<Device>('web')
  const [reauthorizing, setReauthorizing] = useState(false)
  // Session IDs and QR rendering are deliberately component-local: a refresh starts a fresh authorization.
  const [session, setSession] = useState<{ sessionId: string; expiresAt: string } | null>(null)
  const [qrImageState, setQrImageState] = useState<'loading' | 'loaded' | 'failed'>('loading')
  const summary = useQuery({ queryKey: connectionKeys.summary, queryFn: getConnectionSummary })
  const qr = useQuery({
    queryKey: connectionKeys.qrSession(session?.sessionId ?? ''),
    queryFn: () => getQRSession(session!.sessionId),
    // Load the protected QR image before polling: a terminal poll can clear the
    // short-lived challenge while the browser is still fetching the image.
    enabled: Boolean(session && qrImageState === 'loaded'),
    refetchInterval: (query) => qrRefetchInterval(query.state.data?.status),
  })
  const create = useMutation({ mutationFn: createQRSession })
  const complete = useMutation({
    mutationFn: completeQRSession,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: connectionKeys.summary })
      setSession(null)
    },
  })
  const completedSession = useRef<string | null>(null)

  function startAuthorization() {
    complete.reset()
    completedSession.current = null
    setQrImageState('loading')
    setSession(null)
    create.mutate(device, {
      onSuccess: (created) => {
        completedSession.current = null
        setReauthorizing(false)
        setQrImageState('loading')
        setSession({ sessionId: created.sessionId, expiresAt: created.expiresAt })
      },
    })
  }
  const test = useMutation({
    mutationFn: testConnection,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: connectionKeys.summary }) },
  })
  const refreshProfile = useMutation({
    mutationFn: refreshAccountProfile,
    onSuccess: async (updated) => {
      queryClient.setQueryData<ConnectionSummary>(connectionKeys.summary, { configured: true, connection: updated })
    },
  })

  useEffect(() => {
    if (qr.data?.status === 'allowed' && session && !complete.isPending
      && !complete.isSuccess && !complete.isError && completedSession.current !== session.sessionId) {
      // Mark before mutating so an effect re-run cannot submit completion twice.
      completedSession.current = session.sessionId
      complete.mutate(session.sessionId)
    }
  }, [qr.data?.status, session, complete])

  const connection = summary.data?.connection
  const error = summary.error ?? create.error ?? qr.error ?? complete.error ?? test.error ?? refreshProfile.error
  const activeQR = qr.data
  const terminalQR = activeQR && ['expired', 'canceled', 'failed'].includes(activeQR.status)
  const busy = create.isPending || complete.isPending

  return (
    <section className="connections-page">
      <div className="space-y-2">
        <p className="text-sm text-muted">管理后台 / 网盘授权</p>
        <h1 className="text-3xl font-semibold">115 连接管理</h1>
        <p className="max-w-2xl text-sm leading-6 text-muted">管理此实例使用的 115 账号。授权凭据保存在服务端，不会发送到浏览器。</p>
      </div>

      {error && <Alert status="danger"><Alert.Indicator /><Alert.Content><Alert.Title>操作未完成</Alert.Title><Alert.Description>{describeConnectionError(error)}</Alert.Description></Alert.Content></Alert>}
      {summary.isLoading && <p role="status" className="text-sm text-muted">正在读取连接状态…</p>}
      {summary.isError && <Button variant="secondary" onPress={() => void summary.refetch()}>重新加载连接状态</Button>}

      {!summary.isLoading && !summary.isError && (
        <Card render={(props) => <section {...props} />} className="connection-card" aria-labelledby="current-connection-title">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h2 id="current-connection-title" className="text-lg font-semibold">当前连接</h2>
              <p className="mt-1 text-sm text-muted">{connection ? '连接摘要与最近验证结果' : '尚未授权 115 网盘'}</p>
            </div>
            {connection && <Chip variant="soft">{connectionStatus(connection.status)}</Chip>}
          </div>
          {connection && <div className="connection-details">
            <div><span>账号</span><strong>{connection.externalAccountId}</strong></div>
            <div><span>显示名称</span><strong>{connection.name}</strong></div>
            <div><span>最近更新</span><strong>{new Date(connection.updatedAt).toLocaleString()}</strong></div>
            <div><span>连接 ID</span><strong className="break-all">{connection.id}</strong></div>
          </div>}
          {connection && <div className="mt-5 rounded-xl bg-surface-secondary p-4" aria-labelledby="account-profile-title">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 id="account-profile-title" className="text-sm font-semibold">115 账号资料</h3>
                <p className="mt-1 text-xs text-muted">授权时获取，可手动刷新；缺失字段显示为未知。</p>
              </div>
              <Button variant="secondary" isPending={refreshProfile.isPending} onPress={() => refreshProfile.mutate()}>
                刷新账号信息
              </Button>
            </div>
            <div className="mt-4 flex items-center gap-3">
              <Avatar size="lg" color="accent" variant="soft" aria-label="115 账号头像">
                <Avatar.Image src={safeAvatarUrl(connection.profile.avatarUrl)} alt="" referrerPolicy="no-referrer" />
                <Avatar.Fallback>{(connection.profile.accountName || connection.name || '115').slice(0, 1)}</Avatar.Fallback>
              </Avatar>
              <div><p className="font-semibold">{connection.profile.accountName || connection.name || '未知'}</p><p className="text-xs text-muted">UID {connection.externalAccountId} · Cookie 授权</p></div>
            </div>
            <dl className="mt-4 grid gap-3 sm:grid-cols-2">
              <div><dt className="text-xs text-muted">账号名</dt><dd className="mt-1 text-sm font-medium">{connection.profile.accountName || connection.name || '未知'}</dd></div>
              <div><dt className="text-xs text-muted">授权方式</dt><dd className="mt-1 text-sm font-medium">Cookie 扫码</dd></div>
              <div>
                <dt className="text-xs text-muted">会员</dt>
                <dd className="mt-1 text-sm font-medium">
                  {formatMembershipName(connection.profile.membershipName)}
                  {connection.profile.membershipExpiresAt && ` · 到期 ${new Date(connection.profile.membershipExpiresAt).toLocaleDateString()}`}
                </dd>
              </div>
              <div><dt className="text-xs text-muted">已用容量</dt><dd className="mt-1 text-sm font-medium">{formatBytes(connection.profile.storageUsedBytes)}</dd></div>
              <div><dt className="text-xs text-muted">总容量</dt><dd className="mt-1 text-sm font-medium">{formatBytes(connection.profile.storageTotalBytes)}</dd></div>
              <div><dt className="text-xs text-muted">剩余容量</dt><dd className="mt-1 text-sm font-medium">{formatBytes(connection.profile.storageFreeBytes)}</dd></div>
              <div><dt className="text-xs text-muted">已登录设备</dt><dd className="mt-1 text-sm font-medium">{connection.profile.deviceCount == null ? '未知' : `${connection.profile.deviceCount} 台`}</dd></div>
              <div><dt className="text-xs text-muted">资料更新时间</dt><dd className="mt-1 text-sm font-medium">{connection.profile.retrievedAt && Date.parse(connection.profile.retrievedAt) > 0 ? new Date(connection.profile.retrievedAt).toLocaleString() : '未知'}</dd></div>
            </dl>
            {storagePercent(connection.profile.storageUsedBytes, connection.profile.storageTotalBytes) != null && <ProgressBar className="mt-4" value={storagePercent(connection.profile.storageUsedBytes, connection.profile.storageTotalBytes)} aria-label="网盘容量使用进度">
              <div className="mb-1 flex justify-between text-xs text-muted"><Label>容量使用</Label><ProgressBar.Output /></div>
              <ProgressBar.Track><ProgressBar.Fill /></ProgressBar.Track>
            </ProgressBar>}
          </div>}
          {connection && <div className="mt-5">
            <h3 className="text-sm font-semibold">能力检查</h3>
            {connection.capabilities.length ? <ul className="mt-2 grid gap-2 sm:grid-cols-2">
              {connection.capabilities.map((capability) => <li key={capability.name} className="capability-row">
                <span>{capabilityName(capability.name)}</span><strong>{capabilityStatus(capability.status)}</strong>
                {capability.detail && <small>{capability.detail}</small>}
              </li>)}
            </ul> : <p className="mt-2 text-sm text-muted">尚无能力检查结果。</p>}
          </div>}
          {connection && <div className="mt-5 flex flex-wrap gap-2">
            <Button variant="secondary" isPending={test.isPending} onPress={() => test.mutate()}>只读测试连接</Button>
            <Button onPress={() => setReauthorizing(true)} isPending={busy}>重新授权</Button>
          </div>}
          {test.data?.message && <p role="status" className="mt-3 text-sm text-muted">{test.data.message}{test.data.upstreamCode ? `（115 错误码 ${test.data.upstreamCode}）` : ''}</p>}
        </Card>
      )}

      {!summary.isLoading && !summary.isError && (!connection || reauthorizing) && <Card render={(props) => <section {...props} />} className="connection-card" aria-labelledby="authorize-title">
        <h2 id="authorize-title" className="text-lg font-semibold">{connection ? '重新授权 115 网盘' : '授权 115 网盘'}</h2>
        <p className="mt-1 text-sm text-muted">选择扫码登录的设备类型，然后使用 115 扫码确认。</p>
        <RadioGroup className="mt-5" name="115-device" value={device} onChange={(value) => {
          const option = deviceOptions.find((item) => item.value === value)
          if (option) setDevice(option.value)
        }} orientation="horizontal" variant="secondary">
          <Label>设备方式</Label>
          <div className="device-options">
            {deviceOptions.map((option) => <Radio key={option.value} value={option.value} className="device-option">
              <Radio.Content className="device-option__content"><Radio.Control><Radio.Indicator /></Radio.Control>{option.label}</Radio.Content>
            </Radio>)}
          </div>
        </RadioGroup>
        {device === 'web' && <div className="web-device-warning" role="note">
          <strong>Web 登录提示</strong>
          <p>Web 设备登录可能影响同账号现有的网页登录状态，也可能触发 115 的 IP 登录异常。若希望减少对常用登录的影响，可改用支付宝或微信小程序。</p>
        </div>}
        <div className="mt-5 flex flex-wrap gap-2">
          <Button isPending={create.isPending} onPress={startAuthorization}>{connection ? '生成重新授权二维码' : '生成扫码二维码'}</Button>
          {connection && <Button variant="secondary" onPress={() => setReauthorizing(false)}>取消</Button>}
        </div>
      </Card>}

      {session && <Card render={(props) => <section {...props} />} className="connection-card qr-session" aria-labelledby="qr-title">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div><h2 id="qr-title" className="text-lg font-semibold">扫码授权</h2><p className="mt-1 text-sm text-muted">{activeQR ? qrStatusLabels[activeQR.status] : '正在获取扫码状态…'}</p></div>
          <Button variant="tertiary" onPress={() => setSession(null)}>结束本次扫码</Button>
        </div>
        {!terminalQR && <div className="qr-image-frame">
          <img
            src={qrImagePath(session.sessionId)}
            alt="115 登录二维码"
            onLoad={() => setQrImageState('loaded')}
            onError={() => setQrImageState('failed')}
          />
        </div>}
        {qrImageState === 'failed' && <p role="alert" className="mt-3 text-sm text-danger">二维码图片加载失败，请重新生成二维码。</p>}
        {activeQR?.message && <p role="status" className="mt-3 text-sm text-muted">{activeQR.message}{activeQR.upstreamCode ? `（115 错误码 ${activeQR.upstreamCode}）` : ''}</p>}
        <p className="mt-3 text-xs text-muted">有效期至 {new Date(session.expiresAt).toLocaleString()}。刷新页面后需重新发起扫码。</p>
        {terminalQR && <Button className="mt-4" onPress={startAuthorization}>重新生成二维码</Button>}
      </Card>}
    </section>
  )
}
