import {
  Breadcrumbs,
  Button,
  Card,
  Chip,
  Link as HeroLink,
  ProgressBar,
} from "@heroui/react";
import {
  Bell,
  ChevronDown,
  ChevronRight,
  Cloud,
  ExternalLink,
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
import { useNavigate } from "react-router";

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
  { heading: "工作台", items: [{ label: "总览", icon: Home, href: "/" }] },
  {
    heading: "资源管理",
    items: [
      { label: "网盘浏览", icon: Folder, href: "/cloud-drive" },
      { label: "媒体库", icon: Film, href: "/libraries" },
      { label: "STRM 管理", icon: Link, href: "/strm" },
      { label: "海报墙", icon: Image },
    ],
  },
  {
    heading: "任务与维护",
    items: [
      { label: "任务中心", icon: ListVideo, href: "/tasks" },
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
  const navigate = useNavigate();
  const auth = useAuth();
  const account = useAccountDetails();
  const isTaskCenter = currentPage === "任务中心" || currentPage === "定时计划";
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
      className={`app-layout${currentPage === "网盘浏览" ? " cloud-drive-shell" : currentPage === "媒体库" ? " media-library-shell" : ""}`}
    >
      <aside className="app-sidebar" aria-label="主导航">
        <div className="app-brand">
          <svg
            className="app-brand-mark"
            viewBox="0 0 32 32"
            aria-hidden="true"
          >
            <defs>
              <linearGradient
                id="brand-mark-gradient"
                x1="0"
                y1="0"
                x2="1"
                y2="1"
              >
                <stop offset="0" stopColor="var(--surface-tertiary)" />
                <stop offset="0.55" stopColor="var(--accent)" />
                <stop offset="1" stopColor="var(--focus)" />
              </linearGradient>
            </defs>
            <path
              d="M5 5.5C5 2.6 7.5 1.5 10.1 2.7l17.2 8.2c3.6 1.7 3.6 7.6 0 9.3L10.1 28.4C7.5 29.6 5 28.5 5 25.6z"
              fill="url(#brand-mark-gradient)"
            />
            <path
              d="M12 10.7c0-1.2 1.2-1.8 2.3-1.2l8.2 4.5c1.2.7 1.2 2.4 0 3.1l-8.2 4.5c-1.1.6-2.3 0-2.3-1.2z"
              fill="var(--accent-foreground)"
            />
          </svg>
          <strong>115 Media Hub</strong>
        </div>
        <nav>
          {navigation.map((group) => (
            <div className="nav-group" key={group.heading}>
              <p>{group.heading}</p>
              <ul className="nav-list">
                {group.items.map(({ label, icon: Icon, href }) => {
                  const current =
                    label === currentPage ||
                    (label === "任务中心" && isTaskCenter);
                  const link = href ? (
                    <div className="flex min-h-11 items-center px-4">
                      <HeroLink
                        className="flex items-center gap-[18px] whitespace-nowrap"
                        href={href}
                        {...(current
                          ? {
                              "aria-current":
                                label === currentPage
                                  ? ("page" as const)
                                  : ("location" as const),
                            }
                          : {})}
                      >
                        <Icon className="size-5 shrink-0" aria-hidden="true" />
                        {label}
                      </HeroLink>
                    </div>
                  ) : (
                    <div className="w-full pl-1">
                      <Button
                        variant="ghost"
                        className="w-full justify-start gap-[18px]"
                        isDisabled
                      >
                        <Icon className="size-5 shrink-0" aria-hidden="true" />
                        {label}
                      </Button>
                    </div>
                  );
                  return (
                    <li
                      key={label}
                      className={current ? "nav-current" : undefined}
                      title={href ? undefined : "功能尚未开放"}
                    >
                      {link}
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
          <div className="sidebar-bottom">
            <ul className="nav-list">
              <li
                className={currentPage === "设置" ? "nav-current" : undefined}
              >
                <div className="flex min-h-11 items-center px-4">
                  <HeroLink
                    className="flex items-center gap-[18px] whitespace-nowrap"
                    href="/settings"
                    {...(currentPage === "设置"
                      ? { "aria-current": "page" as const }
                      : {})}
                  >
                    <Settings className="size-5 shrink-0" aria-hidden="true" />
                    设置
                  </HeroLink>
                </div>
              </li>
            </ul>
            <Card
              role="region"
              className="sidebar-provider-card"
              aria-label="115 网盘授权"
            >
              <Card.Content>
                <HeroLink
                  className="sidebar-provider-heading w-full"
                  href="/settings?tab=account"
                >
                  <Cloud aria-hidden="true" className="text-accent" />
                  <strong>网盘授权</strong>
                  <ChevronRight aria-hidden="true" />
                </HeroLink>
                <div className="sidebar-provider-body">
                  <div role="status" aria-live="polite">
                    <Chip
                      size="sm"
                      variant="soft"
                      color={
                        account.data?.status === "valid" && !account.isError
                          ? "success"
                          : account.data?.status === "invalid" ||
                              account.isError
                            ? "danger"
                            : "default"
                      }
                    >
                      <span
                        aria-hidden="true"
                        className="inline-block size-2 rounded-full bg-current"
                      />
                      {account.isPending
                        ? "正在检查授权"
                        : account.isError
                          ? "无法确认授权"
                          : account.data.status === "valid"
                            ? "授权有效"
                            : account.data.status === "invalid"
                              ? "授权失效"
                              : "尚未授权"}
                    </Chip>
                  </div>
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
                          <ProgressBar
                            aria-label="网盘空间使用率"
                            value={capacityMetrics.percent}
                          >
                            <ProgressBar.Track>
                              <ProgressBar.Fill />
                            </ProgressBar.Track>
                          </ProgressBar>
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
                  <div className="sidebar-provider-action">
                    <Button
                      fullWidth
                      size="sm"
                      variant="secondary"
                      onPress={() => {
                        void navigate("/settings?tab=account");
                      }}
                    >
                      {account.data?.status === "invalid" && !account.isError
                        ? "重新授权"
                        : "管理授权"}
                      <ExternalLink aria-hidden="true" />
                    </Button>
                  </div>
                </div>
              </Card.Content>
            </Card>
          </div>
        </nav>
      </aside>
      <div className="app-main">
        <header className="app-topbar">
          <nav aria-label="面包屑">
            <Breadcrumbs className="gap-3" separator="/">
              <Breadcrumbs.Item href="/" aria-label="返回总览">
                <Home aria-hidden="true" className="size-5" />
              </Breadcrumbs.Item>
              <Breadcrumbs.Item>
                {currentPage === "网盘浏览" ||
                currentPage === "媒体库" ||
                currentPage === "STRM 管理"
                  ? "资源管理"
                  : isTaskCenter
                    ? "任务与维护"
                    : "工作台"}
              </Breadcrumbs.Item>
              <Breadcrumbs.Item>{currentPage}</Breadcrumbs.Item>
            </Breadcrumbs>
          </nav>
          <div className="top-actions">
            {!isTaskCenter && (
              <span>
                <Button variant="ghost" isDisabled>
                  <Play aria-hidden="true" />
                  运行任务 <b>3</b>
                  <small>示例</small>
                </Button>
              </span>
            )}
            <ThemeToggle />
            <span>
              <Button
                variant="ghost"
                isIconOnly
                isDisabled
                aria-label="通知功能尚未开放"
              >
                <Bell aria-hidden="true" />
                <i />
              </Button>
            </span>
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
