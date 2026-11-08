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

import { AuthError } from "@/features/auth/auth-error";
import { useAuth } from "@/features/auth/auth-provider";
import { ThemeToggle } from "@/features/auth/theme-toggle";

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
    <div className="app-layout">
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
            <button className="nav-item" disabled title="功能尚未开放">
              <Settings aria-hidden="true" />
              设置
            </button>
            <div className="sidebar-example">
              <Cloud aria-hidden="true" />
              <strong>网盘授权</strong>
              <ChevronRight className="sidebar-chevron" aria-hidden="true" />
              <div className="sidebar-example-body">
                <span className="example-badge">● 授权有效 · 示例数据</span>
                <p>账号：138****5678</p>
                <p>已用 7.23 TB / 15 TB</p>
                <div className="sidebar-capacity">
                  <div className="progress-track">
                    <span />
                  </div>
                  48.2%
                </div>
                <button disabled>管理授权 ↗</button>
                <small>示例数据 · 功能尚未开放</small>
              </div>
            </div>
          </div>
        </nav>
      </aside>
      <div className="app-main">
        <header className="app-topbar">
          <div className="breadcrumbs">
            <Home aria-hidden="true" /> <span>/</span> 工作台 <span>/</span>{" "}
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
