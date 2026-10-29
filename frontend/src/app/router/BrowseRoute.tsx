import { lazy, Suspense } from 'react'

const BrowsePage = lazy(async () => {
  const module = await import('../../features/browse/BrowsePage')
  return { default: module.BrowsePage }
})

export function BrowseRoute() {
  return <Suspense fallback={<p className="p-6 text-sm text-muted">正在打开网盘浏览…</p>}><BrowsePage /></Suspense>
}
