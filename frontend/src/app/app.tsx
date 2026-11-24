import { Button, RouterProvider } from "@heroui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import {
  BrowserRouter,
  Navigate,
  Outlet,
  Route,
  Routes,
  useNavigate,
} from "react-router";

import { AuthBoundary } from "@/features/auth/auth-boundary";
import { AuthProvider, useAuth } from "@/features/auth/auth-provider";
import { LoginPage } from "@/features/auth/login-page";
import { SetupPage } from "@/features/auth/setup-page";
import { ThemeToggle } from "@/features/auth/theme-toggle";
import { CloudDrivePage } from "@/features/cloud-drive/cloud-drive-page";
import { DashboardPage } from "@/features/dashboard/dashboard-page";
import { LibraryExecutionPreviewPage } from "@/features/media-library/library-execution-preview-page";
import { LibraryFullSyncPage } from "@/features/media-library/library-full-sync-page";
import { LibraryIncrementalSyncPage } from "@/features/media-library/library-incremental-sync-page";
import {
  LibraryDetailLayout,
  LibraryDetailPage,
  LibraryFormPage,
  LibraryListPage,
} from "@/features/media-library/library-page";
import { SettingsPage } from "@/features/settings/settings-page";
import { StrmDetailPage } from "@/features/strm-management/strm-detail-page";
import { StrmListPage } from "@/features/strm-management/strm-list-page";
import { ScheduleEditorPage } from "@/features/task-center/schedule-editor-page";
import { ScheduleListPage } from "@/features/task-center/schedule-list-page";
import { TaskDetailPage } from "@/features/task-center/task-detail-page";
import { TaskEventListener } from "@/features/task-center/task-event-listener";
import { TaskOverviewPage } from "@/features/task-center/task-overview-page";

import { AppShell } from "./app-shell";

function AppRouter({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  return (
    <RouterProvider
      navigate={(path, options) => {
        void navigate(path, options);
      }}
    >
      {children}
    </RouterProvider>
  );
}

function NotFound() {
  const navigate = useNavigate();
  return (
    <main className="page-shell">
      <h1>页面不存在</h1>
      <Button
        variant="secondary"
        onPress={() => {
          void navigate("/");
        }}
      >
        返回首页
      </Button>
    </main>
  );
}

function AuthenticatedTaskEvents() {
  const { data } = useAuth();
  return data?.authenticated ? <TaskEventListener /> : null;
}

export function App() {
  const [queryClient] = useState(() => new QueryClient());
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppRouter>
          <AuthProvider>
            <AuthenticatedTaskEvents />
            <Routes>
              <Route element={<AuthBoundary />}>
                <Route
                  path="/setup"
                  element={
                    <>
                      <ThemeToggle />
                      <SetupPage />
                    </>
                  }
                />
                <Route
                  path="/login"
                  element={
                    <>
                      <ThemeToggle />
                      <LoginPage />
                    </>
                  }
                />
                <Route
                  path="/"
                  element={
                    <AppShell currentPage="总览">
                      <DashboardPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/cloud-drive"
                  element={
                    <AppShell currentPage="网盘浏览">
                      <CloudDrivePage />
                    </AppShell>
                  }
                />
                <Route
                  path="/libraries"
                  element={
                    <AppShell currentPage="媒体库">
                      <LibraryListPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/libraries/new"
                  element={
                    <AppShell currentPage="媒体库">
                      <LibraryFormPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/libraries/:id"
                  element={
                    <AppShell currentPage="媒体库">
                      <LibraryDetailLayout />
                    </AppShell>
                  }
                >
                  <Route index element={<LibraryDetailPage />} />
                  <Route path="full-sync" element={<LibraryFullSyncPage />} />
                  <Route
                    path="full-sync/execution-preview"
                    element={<LibraryExecutionPreviewPage />}
                  />
                  <Route
                    path="incremental-sync"
                    element={<LibraryIncrementalSyncPage />}
                  />
                </Route>
                <Route
                  path="/libraries/:id/edit"
                  element={
                    <AppShell currentPage="媒体库">
                      <LibraryFormPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/strm"
                  element={
                    <AppShell currentPage="STRM 管理">
                      <main className="mx-auto w-full min-w-0 max-w-[1600px] px-3 pt-4 pb-9 sm:px-5 sm:pt-5 sm:pb-12">
                        <Outlet />
                      </main>
                    </AppShell>
                  }
                >
                  <Route index element={<StrmListPage />} />
                  <Route path="files/:id" element={<StrmDetailPage />} />
                  <Route
                    path="check-results"
                    element={<Navigate to="/strm" replace />}
                  />
                  <Route
                    path="cleanup-preview"
                    element={<Navigate to="/strm" replace />}
                  />
                </Route>
                <Route
                  path="/tasks"
                  element={
                    <AppShell currentPage="任务中心">
                      <TaskOverviewPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/tasks/:id"
                  element={
                    <AppShell currentPage="任务中心">
                      <TaskDetailPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/tasks/schedules"
                  element={
                    <AppShell currentPage="定时计划">
                      <ScheduleListPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/tasks/schedules/new"
                  element={
                    <AppShell currentPage="定时计划">
                      <ScheduleEditorPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/tasks/schedules/:id/edit"
                  element={
                    <AppShell currentPage="定时计划">
                      <ScheduleEditorPage />
                    </AppShell>
                  }
                />
                <Route
                  path="/settings"
                  element={
                    <AppShell currentPage="设置">
                      <SettingsPage />
                    </AppShell>
                  }
                />
                <Route
                  path="*"
                  element={
                    <>
                      <ThemeToggle />
                      <NotFound />
                    </>
                  }
                />
              </Route>
            </Routes>
          </AuthProvider>
        </AppRouter>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
