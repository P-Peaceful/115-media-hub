import { Button, Card, Spinner } from "@heroui/react";
import { Navigate, Outlet, useLocation, useSearchParams } from "react-router";

import { ApiError } from "@/lib/api/errors";

import { useAuth } from "./auth-provider";
import { safeReturnPath } from "./return-path";

export function AuthBoundary() {
  const auth = useAuth();
  const location = useLocation();
  const [search] = useSearchParams();
  if (auth.isPending || auth.error) {
    return (
      <main className="auth-state">
        <Card className="auth-state-card">
          <h1>{auth.error ? "暂时无法连接服务" : "正在连接媒体中心"}</h1>
          {auth.error ? (
            <>
              <p role="alert">
                {auth.error instanceof ApiError
                  ? auth.error.message
                  : "加载失败，请稍后重试。"}
              </p>
              <Button
                onPress={() => {
                  void auth.retry();
                }}
              >
                重新连接
              </Button>
            </>
          ) : (
            <div role="status">
              <Spinner aria-hidden="true" />
              <span>正在确认登录状态…</span>
            </div>
          )}
        </Card>
      </main>
    );
  }
  if (!auth.data) return null;
  if (!auth.data.initialized && location.pathname !== "/setup")
    return <Navigate to="/setup" replace />;
  if (
    auth.data.initialized &&
    !auth.data.authenticated &&
    location.pathname !== "/login"
  ) {
    const returnTo = safeReturnPath(
      location.pathname + location.search + location.hash,
    );
    return (
      <Navigate
        to={`/login${returnTo === "/" ? "" : `?return_to=${encodeURIComponent(returnTo)}`}`}
        replace
      />
    );
  }
  if (
    auth.data.authenticated &&
    ["/login", "/setup"].includes(location.pathname)
  ) {
    return <Navigate to={safeReturnPath(search.get("return_to"))} replace />;
  }
  return <Outlet />;
}
