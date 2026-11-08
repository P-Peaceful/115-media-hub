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
  Shield,
  UploadCloud,
  Zap,
} from "lucide-react";

import folderArtwork from "@/assets/media-folder.svg";
import { useReadiness } from "@/features/health/use-readiness";
import { ApiError } from "@/lib/api/errors";

import { formatUptime, useSystemStatus } from "./system-status";

export function DashboardPage() {
  const health = useReadiness();
  const system = useSystemStatus();
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
          <div role="status" aria-live="polite" aria-atomic="true">
            <Chip
              color={
                health.isError
                  ? "danger"
                  : health.isSuccess
                    ? "success"
                    : "default"
              }
              variant="soft"
            >
              {label}
            </Chip>
            <p className="system-description">{description}</p>
            {health.isError &&
              health.error instanceof ApiError &&
              health.error.requestId && (
                <p className="system-description">
                  请求编号：{health.error.requestId}
                </p>
              )}
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
        <section className="dashboard-card">
          <h2>
            <Cloud />
            115 连接状态 <span className="example-label">示例数据</span>
          </h2>
          <span className="example-badge">授权有效</span>
          <div className="metric-line">
            <span>
              <Shield />
              授权状态
            </span>
            <strong>有效（剩余 356 天）</strong>
          </div>
          <div className="metric-line">
            <span>
              <RefreshCw />
              最近检测
            </span>
            <strong>2025-01-26 10:24</strong>
          </div>
        </section>
        <section className="dashboard-card">
          <h2>
            <Database />
            网盘容量 <span className="example-label">示例数据</span>
          </h2>
          <p className="capacity">
            <strong>7.23 TB</strong> / 15 TB
          </p>
          <div className="capacity-progress">
            <div className="progress-track">
              <span />
            </div>
            <span>48.2%</span>
          </div>
          <p className="capacity-remaining">剩余 7.77 TB</p>
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
                <Shield aria-hidden="true" />
                <span className="alert-copy">
                  <strong>115 授权即将失效</strong>
                  <span>当前授权剩余 7 天，请及时重新授权。</span>
                </span>
                <button disabled>去处理</button>
              </p>
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
