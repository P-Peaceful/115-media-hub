import { createBrowserRouter } from 'react-router'
import { FoundationPage, NotFoundPage, ErrorPage } from './FoundationPages'
import { AppLayout } from '../layouts/AppLayout'
import { AuthGuard } from '../../features/auth/AuthGuard'
import { LoginPage } from '../../features/auth/LoginPage'
import { ConnectionsPage } from '../../features/connections/ConnectionsPage'
import { BrowseRoute } from './BrowseRoute'

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage />, errorElement: <ErrorPage /> },
  { element: <AuthGuard />, errorElement: <ErrorPage />, children: [
    { element: <AppLayout />, children: [
      { path: '/', element: <FoundationPage /> },
      { path: '/connections', element: <ConnectionsPage /> },
      { path: '/browse', element: <BrowseRoute /> },
      { path: '*', element: <NotFoundPage /> },
    ] },
  ] },
])
