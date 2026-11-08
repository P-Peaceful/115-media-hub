import { Button } from "@heroui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { BrowserRouter, Route, Routes, useNavigate } from "react-router";

import { AuthBoundary } from "@/features/auth/auth-boundary";
import { AuthProvider } from "@/features/auth/auth-provider";
import { LoginPage } from "@/features/auth/login-page";
import { SetupPage } from "@/features/auth/setup-page";
import { ThemeToggle } from "@/features/auth/theme-toggle";
import { DashboardPage } from "@/features/dashboard/dashboard-page";

import { AppShell } from "./app-shell";

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

export function App() {
  const [queryClient] = useState(() => new QueryClient());
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
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
      </BrowserRouter>
    </QueryClientProvider>
  );
}
