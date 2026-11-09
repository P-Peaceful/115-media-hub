import { Button, Chip } from "@heroui/react";
import {
  Activity,
  AlertTriangle,
  Box,
  ChevronRight,
  CircleAlert,
  CircleX,
  Clock,
  Cloud,
  Database,
  Film,
  FolderOpen,
  Layers,
  Link,
  Play,
  RefreshCw,
  Search,
  ShieldCheck,
  UploadCloud,
  Zap,
} from "lucide-react";
import { Link as RouterLink } from "react-router";

import folderArtwork from "@/assets/media-folder.svg";
import { useReadiness } from "@/features/health/use-readiness";
import {
  formatBytes,
  formatCheckedAt,
  getCapacityMetrics,
  useAccountDetails,
} from "@/features/settings/account-details";
import { ApiError } from "@/lib/api/errors";

import { formatUptime, useSystemStatus } from "./system-status";

export function DashboardPage() {
  const health = useReadiness();
  const system = useSystemStatus();
  const account = useAccountDetails();
  let label = "正在检查";
  let description = "正在确认服务与数据库的连接状态…";
  if (health.isError) {
    label = "连接失败";
    description =
      health.error instanceof ApiError
        ? health.error.message
        : "检查失败，请稍后重试。";
  } else if (health.isSuccess) {
    label = "服务已就绪";
    description = "后端服务与数据库连接正常。";
  }
  let accountLabel = "尚未授权";
  let accountDescription = "尚未配置 115 账号，请先扫码完成授权。";
  let accountColor: "default" | "warning" | "success" | "danger" = "default";
  let accountDotClass = "";
  let accountStatusText = "未配置";
  if (account.isPending) {
    accountLabel = "正在检查授权…";
    accountDescription = "正在确认当前 115 账号的授权状态。";
    accountStatusText = "检查中…";
  } else if (account.isError) {
    accountLabel = "无法确认授权状态";
    accountDescription = "暂时无法连接 115 服务，请稍后重试。";
    accountColor = "warning";
    accountDotClass = "is-unavailable";
    accountStatusText = "暂时无法确认";
  } else if (account.data.status === "valid") {
    accountLabel = "授权有效";
    accountDescription = "当前账号已授权，可正常使用网盘服务。";
    accountColor = "success";
    accountDotClass = "is-valid";
    accountStatusText = "有效";
  } else if (account.data.status === "invalid") {
    accountLabel = "授权失效";
    accountDescription = "授权已失效，请重新扫码完成授权。";
    accountColor = "danger";
    accountDotClass = "is-invalid";
    accountStatusText = "已失效";
  }
  return (
    <main className="dashboard">
      <section className="dashboard-welcome">
        <div>
          <h1>欢迎使用 115 Media Hub</h1>
          <p>
            基于 115 网盘的媒体管理与 STRM
            生态解决方案，统一管理资源、任务与同步回传。
          </p>
        </div>
        <img src={folderArtwork} alt="" aria-hidden="true" />
      </section>
      <div className="dashboard-grid first-row">
        <section
          className="dashboard-card system-card"
          aria-labelledby="system-title"
        >
          <h2 id="system-title">
            <Box />
            系统状态
          </h2>
          <div
            className="dashboard-status-block"
            role="status"
            aria-live="polite"
            aria-atomic="true"
          >
            <Chip
              className="dashboard-status-badge"
              color={
                health.isError
                  ? "danger"
                  : health.isSuccess
                    ? "success"
                    : "default"
              }
              variant="soft"
            >
              <span
                className={`provider-state-dot ${health.isError ? "is-invalid" : health.isSuccess ? "is-valid" : ""}`}
                aria-hidden="true"
              />
              {label}
            </Chip>
            <p className="dashboard-status-description">{description}</p>
          </div>
          <div className="system-line">
            <span>
              <Clock />
              运行时长
            </span>
            <span>
              {system.data && !system.isError
                ? formatUptime(system.data.uptime_seconds)
                : system.isPending
                  ? "加载中…"
                  : "暂无数据"}
            </span>
          </div>
          <div className="system-line">
            <span>
              <Box />
              版本信息
            </span>
            <span>
              {system.data && !system.isError
                ? system.data.version
                : system.isPending
                  ? "加载中…"
                  : "暂无数据"}
            </span>
          </div>
          {system.isError && (
            <p className="system-description">系统信息获取失败，请重新检查。</p>
          )}
          {health.isError &&
            health.error instanceof ApiError &&
            health.error.requestId && (
              <p className="system-description">
                请求编号：{health.error.requestId}
              </p>
            )}
          <Button
            variant="secondary"
            isPending={health.isFetching || system.isFetching}
            onPress={() => {
              void health.refetch();
              void system.refetch();
            }}
          >
            <RefreshCw aria-hidden="true" className="size-4" />
            {health.isFetching || system.isFetching ? "检查中…" : "重新检查"}
          </Button>
        </section>
        <section
          className="dashboard-card provider-connection-card"
          aria-labelledby="provider-connection-title"
        >
          <h2 id="provider-connection-title">
            <Cloud />
            115 连接状态
          </h2>
          <div
            className="dashboard-status-block"
            role="status"
            aria-live="polite"
            aria-atomic="true"
          >
            <Chip
              className="dashboard-status-badge"
              color={accountColor}
              variant="soft"
            >
              <span
                className={`provider-state-dot ${accountDotClass}`}
                aria-hidden="true"
              />
              {accountLabel}
            </Chip>
            <p className="dashboard-status-description">{accountDescription}</p>
          </div>
          <div className="provider-detail-line">
            <span>
              <ShieldCheck aria-hidden="true" />
              授权状态
            </span>
            <span>{accountStatusText}</span>
          </div>
          <div className="provider-detail-line">
            <span>
              <RefreshCw aria-hidden="true" />
              最近检测
            </span>
            <span>
              {account.isError
                ? "本次检测失败"
                : formatCheckedAt(account.dataUpdatedAt || undefined)}
            </span>
          </div>
          <RouterLink
            className="provider-card-manage"
            to="/settings?tab=account"
            aria-label={
              account.data?.status === "invalid"
                ? "重新授权 115"
                : "管理 115 授权"
            }
          >
            管理授权 <ChevronRight aria-hidden="true" />
          </RouterLink>
        </section>
        <section className="dashboard-card provider-capacity-card">
          <h2>
            <Database />
            <span>网盘容量</span>
            <RouterLink
              className="capacity-details-link"
              to="/settings?tab=account"
            >
              容量详情 <ChevronRight aria-hidden="true" />
            </RouterLink>
          </h2>
          {account.isPending ? (
            <p className="capacity-empty" role="status">
              正在加载容量…
            </p>
          ) : account.isError ? (
            <p className="capacity-empty" role="status">
              容量暂不可用
            </p>
          ) : account.data.status !== "valid" ? (
            <p className="capacity-empty" role="status">
              授权后可查看容量
            </p>
          ) : account.data.space_status !== "available" ||
            !account.data.space ? (
            <p className="capacity-empty" role="status">
              容量信息暂不可用
            </p>
          ) : (
            <CapacitySummary space={account.data.space} />
          )}
        </section>
      </div>
      <div className="dashboard-grid stats-row">
        <Stat
          icon={<Film />}
          title="媒体库统计"
          values={[
            ["1,248", "媒体总数"],
            ["1,186", "已启用"],
            ["62", "已暂停"],
          ]}
        />
        <Stat
          icon={<Link />}
          title="STRM 统计"
          values={[
            ["28,560", "已生成"],
            ["342", "已失效"],
            ["27", "待处理异常"],
          ]}
        />
        <Stat
          icon={<Layers />}
          title="任务摘要"
          values={[
            ["3", "运行中"],
            ["8", "排队中"],
            ["6", "失败"],
          ]}
        />
      </div>
      <div className="dashboard-grid lower-row">
        <section className="dashboard-card">
          <h2>
            <Activity />
            近期活动 <span className="example-label">示例数据</span>
          </h2>
          <ul className="activity-list">
            {[
              [
                "今天 10:24",
                "同步",
                "完成媒体库「电影」的增量同步",
                "新增 12 个文件，更新 3 个文件",
              ],
              [
                "今天 10:12",
                "生成",
                "生成 STRM 文件（电影 / 4K）",
                "共生成 128 个 STRM",
              ],
              [
                "今天 09:56",
                "扫描",
                "完成目录扫描：/我的设备/影视",
                "发现 36 个新增文件",
              ],
              ["今天 09:21", "回传", "回传处理完成", "成功回传 8 个文件"],
              [
                "昨天 22:30",
                "媒体库",
                "媒体库「电视剧」更新完成",
                "新增 24 部剧集",
              ],
            ].map(([time, type, title, detail], index) => (
              <li key={title}>
                <span className="activity-dot" />
                <span className={`activity-icon activity-icon-${index}`}>
                  {index === 0 ? (
                    <RefreshCw />
                  ) : index === 1 ? (
                    <Link />
                  ) : index === 2 ? (
                    <Search />
                  ) : index === 3 ? (
                    <UploadCloud />
                  ) : (
                    <Film />
                  )}
                </span>
                <time>{time}</time>
                <span className="activity-type">{type}</span>
                <div>
                  <strong>{title}</strong>
                  <p>{detail}</p>
                </div>
              </li>
            ))}
          </ul>
        </section>
        <div className="lower-stack">
          <section className="dashboard-card">
            <h2>
              <AlertTriangle />
              异常提醒 <span className="example-label">示例数据</span>
            </h2>
            <div className="alert-list">
              <p>
                <FolderOpen aria-hidden="true" />
                <span className="alert-copy">
                  <strong>目录不可用</strong>
                  <span>目录无法访问，已跳过扫描。</span>
                </span>
                <button disabled>去处理</button>
              </p>
              <p>
                <CircleX aria-hidden="true" />
                <span className="alert-copy">
                  <strong>任务执行失败</strong>
                  <span>STRM 生成任务执行失败。</span>
                </span>
                <button disabled>查看详情</button>
              </p>
            </div>
          </section>
          <section className="dashboard-card">
            <h2>
              <Zap />
              快捷入口
            </h2>
            <div className="shortcut-grid">
              {["新建媒体库", "查看任务", "处理异常"].map((name, index) => (
                <button disabled key={name}>
                  <div className="shortcut-icon">
                    {index === 0 ? (
                      <Layers />
                    ) : index === 1 ? (
                      <Play />
                    ) : (
                      <CircleAlert />
                    )}
                    <ChevronRight />
                  </div>
                  <strong>{name}</strong>
                  <span>功能尚未开放</span>
                </button>
              ))}
            </div>
          </section>
        </div>
      </div>
    </main>
  );
}

function CapacitySummary({
  space,
}: {
  space: { used_bytes: string; total_bytes: string };
}) {
  const metrics = getCapacityMetrics(space);
  return (
    <div className="capacity-summary">
      <div className="capacity-summary-values">
        <p className="capacity">
          <strong>{formatBytes(space.used_bytes)}</strong> /{" "}
          {formatBytes(space.total_bytes)}
        </p>
        {metrics && (
          <span className="capacity-remaining-pill">
            剩余 {formatBytes(metrics.remaining)}
          </span>
        )}
      </div>
      {metrics ? (
        <div className="capacity-progress">
          <div
            className="progress-track"
            role="progressbar"
            aria-label="网盘空间使用率"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={metrics.percent}
          >
            <span style={{ width: `${metrics.percent}%` }} />
          </div>
          <span>{metrics.percentLabel}</span>
        </div>
      ) : (
        <p className="capacity-empty">容量使用率暂不可用</p>
      )}
    </div>
  );
}

function Stat({
  icon,
  title,
  values,
}: {
  icon: React.ReactNode;
  title: string;
  values: string[][];
}) {
  return (
    <section className="dashboard-card">
      <h2>
        {icon}
        {title}
        <span className="example-label">示例数据</span>
      </h2>
      <div className="stat-values">
        {values.map(([value, label]) => (
          <div key={label}>
            <strong>{value}</strong>
            <span>{label}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
