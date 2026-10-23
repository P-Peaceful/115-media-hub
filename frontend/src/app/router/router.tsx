import { createBrowserRouter } from 'react-router'
import { FoundationPage, NotFoundPage, ErrorPage } from './FoundationPages'
import { AppLayout } from '../layouts/AppLayout'

export const router = createBrowserRouter([
  { element: <AppLayout />, errorElement: <ErrorPage />, children: [
    { path: '/', element: <FoundationPage /> },
    { path: '*', element: <NotFoundPage /> },
  ] },
])
