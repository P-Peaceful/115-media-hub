import { Button } from "@heroui/react";
import {
  Bell,
  ChevronDown,
  ChevronRight,
  Cloud,
  FileText,
  Film,
  Folder,
  Home,
  Image,
  Link,
  ListVideo,
  LogOut,
  Play,
  Settings,
  UploadCloud,
  UserRound,
} from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { Link as RouterLink } from "react-router";

import { AuthError } from "@/features/auth/auth-error";
import { useAuth } from "@/features/auth/auth-provider";
import { ThemeToggle } from "@/features/auth/theme-toggle";
import {
  formatBytes,
  getCapacityMetrics,
  maskAccountId,
  useAccountDetails,
} from "@/features/settings/account-details";

const navigation = [
  { heading: "工作台", items: [{ label: "总览", icon: Home, current: true }] },
  {
    heading: "资源管理",
    items: [
      { label: "网盘浏览", icon: Folder },
      { label: "媒体库", icon: Film },
      { label: "STRM 管理", icon: Link },
      { label: "海报墙", icon: Image },
    ],
  },
  {
    heading: "任务与维护",
    items: [
      { label: "任务中心", icon: ListVideo },
      { label: "文件回传", icon: UploadCloud },
      { label: "运行日志", icon: FileText },
    ],
  },
];

export function AppShell({
  children,
  currentPage,
}: {
  children: ReactNode;
  currentPage: string;
}) {
  const auth = useAuth();
  const account = useAccountDetails();
  const capacityMetrics =
    !account.isError &&
    account.data?.status === "valid" &&
    account.data.space_status === "available"
      ? getCapacityMetrics(account.data.space)
      : null;
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function signOut() {
    setPending(true);
    setError(null);
    try {
      await auth.signOut();
    } catch (error) {
      setError(error);
    } finally {
      setPending(false);
    }
  }
  return (
    <div
      className={`app-layout${currentPage === "网盘浏览" ? " cloud-drive-shell" : ""}`}
    >
      <aside className="app-sidebar" aria-label="主导航">
        <div className="app-brand">
          <span className="brand-mark">▶</span> 115 Media Hub
        </div>
        <nav>
          {navigation.map((group) => (
            <div className="nav-group" key={group.heading}>
              <p>{group.heading}</p>
              {group.items.map(({ label, icon: Icon }) =>
                label === currentPage ? (
                  <span
                    className="nav-item active"
                    aria-current="page"
                    key={label}
                  >
                    <Icon aria-hidden="true" />
                    {label}
                  </span>
                ) : label === "总览" || label === "网盘浏览" ? (
                  <RouterLink
                    className="nav-item"
                    to={label === "总览" ? "/" : "/cloud-drive"}
                    key={label}
                  >
                    <Icon aria-hidden="true" />
                    {label}
                  </RouterLink>
                ) : (
                  <button
                    className="nav-item"
                    disabled
                    title="功能尚未开放"
                    key={label}
                  >
                    <Icon aria-hidden="true" />
                    {label}
                  </button>
                ),
              )}
            </div>
          ))}
          <div className="sidebar-bottom">
            <RouterLink
              className={`nav-item${currentPage === "设置" ? " active" : ""}`}
              to="/settings"
              aria-current={currentPage === "设置" ? "page" : undefined}
            >
              <Settings aria-hidden="true" />
              设置
            </RouterLink>
            <section
              className="sidebar-provider-card"
              aria-label="115 网盘授权"
            >
              <RouterLink
                className="sidebar-provider-heading"
                to="/settings?tab=account"
              >
                <Cloud aria-hidden="true" />
                <strong>网盘授权</strong>
                <ChevronRight aria-hidden="true" />
              </RouterLink>
              <div className="sidebar-provider-body">
                <p
                  className={`sidebar-provider-status ${account.isPending || account.isError || account.data?.status !== "valid" ? "is-not-valid" : "is-valid"}`}
                  role="status"
                  aria-live="polite"
                >
                  <span aria-hidden="true" />
                  {account.isPending
                    ? "正在检查授权"
                    : account.isError
                      ? "无法确认授权"
                      : account.data.status === "valid"
                        ? "授权有效"
                        : account.data.status === "invalid"
                          ? "授权失效"
                          : "尚未授权"}
                </p>
                <p className="sidebar-provider-account">
                  账号：
                  {account.isError
                    ? "暂不可用"
                    : maskAccountId(account.data?.profile?.user_id)}
                </p>
                {account.isError ? (
                  <p className="sidebar-provider-capacity-unavailable">
                    容量暂不可用
                  </p>
                ) : account.data?.status === "valid" &&
                  account.data.space_status === "available" &&
                  account.data.space ? (
                  <>
                    <div className="sidebar-provider-capacity-label">
                      已用 {formatBytes(account.data.space.used_bytes)} /{" "}
                      {formatBytes(account.data.space.total_bytes)}
                    </div>
                    {capacityMetrics ? (
                      <div className="sidebar-provider-capacity-row">
                        <div
                          className="progress-track"
                          role="progressbar"
                          aria-label="网盘空间使用率"
                          aria-valuemin={0}
                          aria-valuemax={100}
                          aria-valuenow={capacityMetrics.percent}
                        >
                          <span
                            style={{
                              width: `${capacityMetrics.percent}%`,
                            }}
                          />
                        </div>
                        <span>{capacityMetrics.percentLabel}</span>
                      </div>
                    ) : (
                      <p className="sidebar-provider-capacity-unavailable">
                        容量使用率暂不可用
                      </p>
                    )}
                  </>
                ) : account.data?.status === "valid" ? (
                  <p className="sidebar-provider-capacity-unavailable">
                    容量信息暂不可用
                  </p>
                ) : null}
                <RouterLink
                  className="sidebar-provider-manage"
                  to="/settings?tab=account"
                >
                  {account.data?.status === "invalid" ? "重新授权" : "管理授权"}
                  <span aria-hidden="true">↗</span>
                </RouterLink>
              </div>
            </section>
          </div>
        </nav>
      </aside>
      <div className="app-main">
        <header className="app-topbar">
          <div className="breadcrumbs">
            <Home aria-hidden="true" /> <span>/</span>{" "}
            {currentPage === "网盘浏览" ? "资源管理" : "工作台"} <span>/</span>{" "}
            <strong>{currentPage}</strong>
          </div>
          <div className="top-actions">
            <button className="running-tasks" disabled title="功能尚未开放">
              <Play aria-hidden="true" />
              运行任务 <b>3</b>
              <small>示例</small>
            </button>
            <ThemeToggle />
            <button
              className="notification-button"
              disabled
              title="通知功能尚未开放"
              aria-label="通知功能尚未开放"
            >
              <Bell aria-hidden="true" />
              <i />
            </button>
            <span className="admin-avatar">
              <UserRound aria-hidden="true" />
            </span>
            <span className="admin-name">管理员</span>
            <ChevronDown className="admin-chevron" aria-hidden="true" />
            <Button
              variant="secondary"
              isPending={pending}
              onPress={() => {
                void signOut();
              }}
            >
              <LogOut aria-hidden="true" className="size-4" />
              {pending ? "正在退出…" : "退出登录"}
            </Button>
            <AuthError error={error} />
          </div>
        </header>
        {children}
      </div>
    </div>
  );
}
