import { Link } from 'react-router'

export function FoundationPage() {
  return (
    <section className="space-y-3">
      <p className="text-sm text-muted">项目基础架构</p>
      <h1 className="text-3xl font-semibold">115-media-hub</h1>
      <p className="text-muted">前端工程已初始化，业务功能尚未接入。</p>
    </section>
  )
}
export function NotFoundPage() {
  return <section className="space-y-3"><h1 className="text-2xl font-semibold">页面不存在</h1><Link to="/" className="underline">返回首页</Link></section>
}
export function ErrorPage() {
  return <main className="p-8"><h1>页面加载失败</h1><a href="/">重新打开首页</a></main>
}
