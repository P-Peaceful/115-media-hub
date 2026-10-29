import { Alert, Breadcrumbs, Button, Checkbox, Chip, ListBox, Pagination, SearchField, Select, Skeleton, Table, Tooltip, Typography } from '@heroui/react'
import { ArrowLeft, ArrowRight, ChevronRight, File, Folder, Plus, RefreshCw, Search } from 'lucide-react'
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { ApiError } from '../../shared/api/client'
import { browseKeys, getDirectory, getDirectoryEntries, type DirectoryEntry, type DirectoryMetadata, type EntryType } from './browse-api'
import './browse.css'

const pageSize = 6
const emptyEntries: DirectoryEntry[] = []
const typeOptions: { value: EntryType; label: string }[] = [
  { value: 'all', label: '全部文件类型' }, { value: 'video', label: '视频' },
  { value: 'folder', label: '文件夹' }, { value: 'image', label: '图片' },
  { value: 'document', label: '文档' }, { value: 'audio', label: '音频' }, { value: 'archive', label: '压缩包' },
]
const typeLabels: Record<string, string> = { folder: '文件夹', video: '视频', image: '图片', subtitle: '字幕', metadata: '元数据', other: '文件' }

function formatSize(size: number, isDirectory: boolean) {
  if (isDirectory) return '未知'
  if (size < 1024) return `${size} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = size / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1 }
  return `${value.toFixed(1)} ${units[unit]}`
}

function formatDate(value: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const parts = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }).formatToParts(date)
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((item) => item.type === type)?.value ?? ''
  return `${part('month')} 月 ${part('day')} 日 ${part('hour')}:${part('minute')}`
}

function EntryIcon({ entry }: { entry: DirectoryEntry }) {
  return entry.isDirectory ? <Folder aria-hidden size={20} /> : <File aria-hidden size={20} />
}

export function BrowsePage() {
  const [params, setParams] = useSearchParams()
  const queryClient = useQueryClient()
  const directoryId = params.get('cid') || '0'
  const query = params.get('q') || ''
  const type = (params.get('type') || 'all') as EntryType
  const page = Math.max(1, Number.parseInt(params.get('page') || '1', 10) || 1)
  const offset = (page - 1) * pageSize
  const [selection, setSelection] = useState<{ scope: string; ids: string[] }>({ scope: '', ids: [] })
  // 站内导航时会先把已知面包屑写入 Query 缓存；只有直接打开/刷新时才向 115 查询目录元数据。
  const directory = useQuery({
    queryKey: browseKeys.directory(directoryId),
    queryFn: () => getDirectory(directoryId),
    staleTime: Infinity,
  })
  const listing = useQuery({ queryKey: browseKeys.entries(directoryId, offset, type, query), queryFn: () => getDirectoryEntries(directoryId, offset, type, query) })
  const entries = listing.data?.entries ?? emptyEntries
  const pageCount = Math.max(1, Math.ceil((listing.data?.total ?? 0) / pageSize))
  const selectionScope = `${directoryId}:${query}:${type}:${page}`
  const selected = selection.scope === selectionScope ? selection.ids : []
  const selectedVisibleCount = entries.filter((entry) => selected.includes(entry.id)).length

  function update(next: { cid?: string; q?: string; type?: EntryType; page?: number }) {
    const nextParams = new URLSearchParams(params)
    if (next.cid !== undefined) nextParams.set('cid', next.cid)
    if (next.q !== undefined) {
      if (next.q) nextParams.set('q', next.q)
      else nextParams.delete('q')
    }
    if (next.type !== undefined) {
      if (next.type === 'all') nextParams.delete('type')
      else nextParams.set('type', next.type)
    }
    const nextPage = next.page ?? 1
    if (nextPage > 1) nextParams.set('page', String(nextPage))
    else nextParams.delete('page')
    setParams(nextParams)
  }

  function toggleEntry(id: string) {
    const current = selection.scope === selectionScope ? selection.ids : []
    setSelection({ scope: selectionScope, ids: current.includes(id) ? current.filter((item) => item !== id) : [...current, id] })
  }

  function openDirectory(entry: DirectoryEntry) {
    if (!entry.isDirectory) return
    // 直达链接的 Stat 失败时仍允许继续浏览；当前目录名称未知，用省略号标记缺失的路径段。
    const currentPath = directory.data?.breadcrumbs ?? (directoryId === '0'
      ? [{ directoryId: '0', name: '我的网盘' }]
      : [{ directoryId: '0', name: '我的网盘' }, { directoryId, name: '…' }])
    const breadcrumbs = [...currentPath, { directoryId: entry.id, name: entry.name }]
    queryClient.setQueryData(browseKeys.directory(entry.id), { directoryId: entry.id, name: entry.name, breadcrumbs })
    update({ cid: entry.id })
  }

  function openBreadcrumb(directoryId: string, index: number, breadcrumbs: DirectoryMetadata['breadcrumbs']) {
    const path = breadcrumbs.slice(0, index + 1)
    const crumb = path[path.length - 1]
    if (!crumb) return
    queryClient.setQueryData(browseKeys.directory(directoryId), { directoryId, name: crumb.name, breadcrumbs: path })
    update({ cid: directoryId })
  }

  function togglePageSelection() {
    if (selectedVisibleCount === entries.length && entries.length > 0) {
      const visible = new Set(entries.map((entry) => entry.id))
      setSelection({ scope: selectionScope, ids: selected.filter((id) => !visible.has(id)) })
    } else {
      setSelection({ scope: selectionScope, ids: [...new Set([...selected, ...entries.map((entry) => entry.id)])] })
    }
  }

  const allVisibleSelected = entries.length > 0 && selectedVisibleCount === entries.length
  const someVisibleSelected = selectedVisibleCount > 0 && !allVisibleSelected
  const parentCrumb = directory.data?.breadcrumbs.length ? directory.data.breadcrumbs[directory.data.breadcrumbs.length - 2] : undefined
  const missingConnection = directoryId === '0' && listing.error instanceof ApiError && listing.error.category === 'connection_missing'
  const breadcrumbs = directory.data?.breadcrumbs ?? [{ directoryId: '0', name: '我的网盘' }]

  return (
    <section className="flex min-w-0 w-full flex-col gap-5">
      <div className="flex items-start justify-between gap-4 max-[760px]:flex-col">
        <div>
          <Typography.Heading level={1}>网盘浏览</Typography.Heading>
          <Typography.Paragraph color="muted" size="sm">浏览当前目录，进入文件夹继续查找。</Typography.Paragraph>
        </div>
        <div className="flex items-center gap-2">
          <Button isDisabled aria-describedby="create-library-note"><Plus size={16} />以当前目录创建媒体库</Button>
          <Typography id="create-library-note" type="body-xs" color="muted">暂未接入</Typography>
        </div>
      </div>

      <div className="grid grid-cols-[220px_minmax(240px,400px)_180px_1fr] items-start gap-3 max-[900px]:grid-cols-2 max-[560px]:grid-cols-1">
        <Select aria-label="选择 115 网盘" isDisabled selectedKey="active">
          <Select.Trigger><Select.Value /></Select.Trigger>
          <Select.Popover><ListBox><ListBox.Item id="active">115 · {directory.data ? '当前账号' : '家庭影音'}</ListBox.Item></ListBox></Select.Popover>
        </Select>
        <SearchField key={query} aria-label="搜索当前目录" onSubmit={(value) => update({ q: value.trim() })}>
          <SearchField.Group><SearchField.SearchIcon><Search size={16} /></SearchField.SearchIcon><SearchField.Input name="q" defaultValue={query} placeholder="搜索当前目录" /></SearchField.Group>
        </SearchField>
        <Select aria-label="文件类型" selectedKey={type} onSelectionChange={(key) => { if (key !== null) update({ type: String(key) as EntryType }) }}>
          <Select.Trigger><Select.Value /></Select.Trigger>
          <Select.Popover><ListBox aria-label="文件类型">{typeOptions.map((item) => <ListBox.Item key={item.value} id={item.value} textValue={item.label}>{item.label}</ListBox.Item>)}</ListBox></Select.Popover>
        </Select>
        <div className="flex justify-end max-[900px]:col-span-2 max-[560px]:col-span-1"><Button variant="secondary" onPress={() => void listing.refetch()}><RefreshCw size={16} />刷新</Button></div>
      </div>

      <Breadcrumbs aria-label="目录路径" separator={<ChevronRight aria-hidden size={14} />}>
        {breadcrumbs.map((crumb, index, items) => index === items.length - 1
          ? <Breadcrumbs.Item key={crumb.directoryId}>{crumb.name}</Breadcrumbs.Item>
          : <Breadcrumbs.Item key={crumb.directoryId} href={`?cid=${encodeURIComponent(crumb.directoryId)}`} onClick={(event) => { event.preventDefault(); openBreadcrumb(crumb.directoryId, index, items) }}>{crumb.name}</Breadcrumbs.Item>)}
      </Breadcrumbs>

      {directory.isError && !directory.data && <Alert status="warning"><Alert.Indicator /><Alert.Content><Alert.Title>无法读取目录路径</Alert.Title><Alert.Description>当前目录内容仍可浏览，路径信息暂不可用。</Alert.Description></Alert.Content><Button size="sm" variant="tertiary" onPress={() => void directory.refetch()}>重试</Button></Alert>}
      {listing.isError && !listing.data && <Alert status="danger"><Alert.Indicator /><Alert.Content><Alert.Title>{missingConnection ? '尚未配置 115 网盘连接' : '目录读取失败'}</Alert.Title><Alert.Description>{missingConnection ? '请先在设置中连接 115 网盘，然后刷新当前页。' : '未能读取当前页内容，请重试。'}</Alert.Description></Alert.Content><Button size="sm" onPress={() => void listing.refetch()}>重试当前页</Button></Alert>}
      {listing.isError && listing.data && <Alert status="warning"><Alert.Indicator /><Alert.Content><Alert.Title>刷新失败</Alert.Title><Alert.Description>仍显示上次成功加载的内容。</Alert.Description></Alert.Content><Button size="sm" variant="tertiary" onPress={() => void listing.refetch()}>重试当前页</Button></Alert>}

      <div className="flex items-center justify-between gap-3 max-[600px]:flex-col max-[600px]:items-start">
        <div className="flex items-center gap-3">
          <Checkbox aria-label="选择当前页所有项目" isSelected={allVisibleSelected} isIndeterminate={someVisibleSelected} onChange={togglePageSelection} />
          <Typography type="body-sm">已选择本页 {selected.length} 项</Typography>
          <Typography type="body-xs" color="muted">选择范围仅限当前页</Typography>
        </div>
        <div className="flex items-center gap-4">
          <Button variant="tertiary" onPress={() => setSelection({ scope: selectionScope, ids: [] })} isDisabled={selected.length === 0}>取消选择</Button>
          <div className="flex items-center gap-2"><Button isDisabled variant="secondary" aria-describedby="generation-note"><Plus size={16} />加入生成范围</Button><Typography id="generation-note" type="body-xs" color="muted">暂未接入</Typography></div>
        </div>
      </div>

      <Table className="browse-table"><Table.ScrollContainer><Table.Content aria-label="当前目录文件" className="browse-table__content">
        <Table.Header>
          <Table.Column isRowHeader className="browse-table__name-column">名称</Table.Column>
          <Table.Column className="browse-table__size-column">大小</Table.Column>
          <Table.Column className="browse-table__date-column">修改时间</Table.Column>
          <Table.Column className="browse-table__type-column">文件类型</Table.Column>
          <Table.Column className="browse-table__library-column">媒体库关联</Table.Column>
          <Table.Column className="browse-table__action-column">操作</Table.Column>
        </Table.Header>
        <Table.Body>
          {listing.isPending && Array.from({ length: pageSize }, (_, index) => <Table.Row key={`loading-${index}`}><Table.Cell><Skeleton /><Skeleton /></Table.Cell><Table.Cell><Skeleton /></Table.Cell><Table.Cell><Skeleton /></Table.Cell><Table.Cell><Skeleton /></Table.Cell><Table.Cell><Skeleton /></Table.Cell><Table.Cell><Skeleton /></Table.Cell></Table.Row>)}
          {!listing.isPending && entries.map((entry) => <BrowseRow key={entry.id} entry={entry} selected={selected.includes(entry.id)} onSelect={() => toggleEntry(entry.id)} onOpen={() => openDirectory(entry)} />)}
          {!listing.isPending && !listing.isError && entries.length === 0 && <Table.Row><Table.Cell colSpan={6}><Typography color="muted" align="center">{query ? '当前目录没有匹配的项目' : '当前目录为空'}</Typography></Table.Cell></Table.Row>}
        </Table.Body>
      </Table.Content></Table.ScrollContainer></Table>

      <div className="flex items-center justify-between gap-3 max-[600px]:flex-col max-[600px]:items-start"><Pagination.Root aria-label="目录分页">
        <Pagination.Summary>{listing.data ? `显示 ${listing.data.total === 0 ? 0 : offset + 1}–${Math.min(offset + entries.length, listing.data.total)} 项，共 ${listing.data.total} 项` : '正在读取当前页…'}</Pagination.Summary>
        <Pagination.Content>
          <Pagination.Item><Pagination.Previous aria-label="上一页" isDisabled={page <= 1} onPress={() => update({ page: page - 1 })}><ArrowLeft aria-hidden size={16} />上一页</Pagination.Previous></Pagination.Item>
          {Array.from({ length: Math.min(pageCount, 3) }, (_, index) => Math.max(1, Math.min(page - 1, pageCount - 2)) + index).map((number) => <Pagination.Item key={number}><Pagination.Link isActive={number === page} aria-label={`第 ${number} 页`} onPress={() => update({ page: number })}>{number}</Pagination.Link></Pagination.Item>)}
          <Pagination.Item><Pagination.Next aria-label="下一页" isDisabled={page >= pageCount} onPress={() => update({ page: page + 1 })}>下一页<ArrowRight aria-hidden size={16} /></Pagination.Next></Pagination.Item>
        </Pagination.Content>
      </Pagination.Root></div>
      <div className="flex items-center justify-between gap-4 max-[600px]:flex-col max-[600px]:items-start">
        {parentCrumb && directory.data ? <Button variant="tertiary" onPress={() => openBreadcrumb(parentCrumb.directoryId, directory.data.breadcrumbs.length - 2, directory.data.breadcrumbs)}><ArrowLeft size={16} />返回上级</Button> : <span />}
        <Typography type="body-xs" color="muted">搜索覆盖当前目录的所有分页，不递归搜索子目录。</Typography>
      </div>
    </section>
  )
}

function TruncatedText({ text, muted = false }: { text: string; muted?: boolean }) {
  return <Tooltip delay={250}>
    <Tooltip.Trigger
      className={`browse-table__text${muted ? ' browse-table__text--muted' : ''}`}
      tabIndex={0}
      render={(props) => <span {...props} role={undefined} />}
    >{text}</Tooltip.Trigger>
    <Tooltip.Content placement="top" className="browse-table__tooltip">{text}</Tooltip.Content>
  </Tooltip>
}

function BrowseRow({ entry, selected, onSelect, onOpen }: { entry: DirectoryEntry; selected: boolean; onSelect: () => void; onOpen: () => void }) {
  const detail = entry.isDirectory ? '文件夹' : entry.fileType === 'video' ? `来源文件 ID ${entry.id}` : entry.fileType === 'image' ? `${entry.name} · 海报图片` : typeLabels[entry.fileType] ?? '文件'
  const fileType = entry.isDirectory ? '文件夹' : `${(entry.name.split('.').pop() || '').toUpperCase()} ${typeLabels[entry.fileType] ?? '文件'}`
  return <Table.Row>
    <Table.Cell><div className="browse-table__entry"><Checkbox aria-label={`${selected ? '取消选择' : '选择'} ${entry.name}`} isSelected={selected} onChange={onSelect} /><EntryIcon entry={entry} /><div className="browse-table__entry-copy"><TruncatedText text={entry.name} /><TruncatedText text={detail} muted /></div></div></Table.Cell>
    <Table.Cell><TruncatedText text={formatSize(entry.size, entry.isDirectory)} /></Table.Cell>
    <Table.Cell><TruncatedText text={formatDate(entry.modifiedAt)} /></Table.Cell>
    <Table.Cell><TruncatedText text={fileType} /></Table.Cell>
    <Table.Cell><Tooltip delay={250}><Tooltip.Trigger className="browse-table__text" tabIndex={0} render={(props) => <span {...props} role={undefined} />}><Chip size="sm" color="default" variant="soft" className="browse-table__association">尚未关联</Chip></Tooltip.Trigger><Tooltip.Content placement="top">尚未关联</Tooltip.Content></Tooltip></Table.Cell>
    <Table.Cell>{entry.isDirectory ? <Button size="sm" variant="tertiary" aria-label={`进入 ${entry.name}`} onPress={onOpen}>进入</Button> : '—'}</Table.Cell>
  </Table.Row>
}
