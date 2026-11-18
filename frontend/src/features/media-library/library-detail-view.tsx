import { Alert, Button, Card, Chip, Link as HeroLink } from "@heroui/react";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  ArrowLeft,
  BadgeInfo,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Clock3,
  FileClock,
  Film,
  Folder,
  Link2,
  ListChecks,
  Pause,
  Pencil,
  Play,
  RefreshCw,
  ScanLine,
  Settings2,
  Timer,
  Zap,
} from "lucide-react";
import type { ReactNode } from "react";
import { Link as RouterLink } from "react-router";

import { PageBanner } from "@/components/page-banner";
import {
  formatUptime,
  useSystemStatus,
} from "@/features/dashboard/system-status";

import type { Library } from "./api";
import { libraryDetailDemo as demo } from "./library-detail-demo";

const typeLabels = {
  movie: "电影",
  tv: "电视剧",
  anime: "动漫",
  documentary: "纪录片",
  mixed: "混合",
};

function DetailCard({
  title,
  icon: Icon,
  children,
  demonstration = false,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
  demonstration?: boolean;
}) {
  return (
    <Card className="h-full">
      <Card.Header>
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          <h2 className="flex items-center gap-3 text-lg font-semibold">
            <Icon aria-hidden="true" className="size-6 text-accent" />
            {title}
          </h2>
          {demonstration && (
            <Chip size="sm" variant="soft">
              演示数据
            </Chip>
          )}
        </div>
      </Card.Header>
      <Card.Content>{children}</Card.Content>
    </Card>
  );
}
function DetailLink({
  to,
  children,
  className,
}: {
  to: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <HeroLink
      href={to}
      {...(className ? { className } : {})}
      render={(props) =>
        "href" in props ? (
          <RouterLink {...props} to={to} />
        ) : (
          <span {...props} />
        )
      }
    >
      {children}
    </HeroLink>
  );
}
function LocalTime({ at, compact = false }: { at: string; compact?: boolean }) {
  const date = new Date(at);
  return (
    <time dateTime={at}>
      {date
        .toLocaleString("zh-CN", {
          ...(compact ? {} : { year: "numeric" }),
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          hour12: false,
        })
        .replaceAll("/", "-")}
    </time>
  );
}

export function LibraryDetailView({
  item,
  onToggle,
  pending,
}: {
  item: Library;
  onToggle: () => void;
  pending: boolean;
}) {
  const system = useSystemStatus();
  const statistics = [
    {
      label: "总数量",
      value: demo.statistics.total,
      icon: Link2,
      color: "text-foreground",
    },
    {
      label: "已生成",
      value: demo.statistics.generated,
      icon: CheckCircle2,
      color: "text-success",
    },
    {
      label: "待处理",
      value: demo.statistics.pending,
      icon: Clock3,
      color: "text-warning",
    },
    {
      label: "失败",
      value: demo.statistics.failed,
      icon: CircleAlert,
      color: "text-danger",
    },
  ];
  return (
    <>
      <PageBanner
        title={
          <>
            <RouterLink
              to="/libraries"
              aria-label="返回媒体库"
              className="mr-3 inline-flex align-middle text-foreground"
            >
              <ArrowLeft />
            </RouterLink>
            {item.name}
          </>
        }
        description={`${typeLabels[item.media_type]}类型媒体库，绑定 115 网盘目录，管理 STRM 输出配置。`}
      />
      <div className="mt-4 rounded-2xl bg-surface p-2 shadow-sm">
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <Button
            variant="primary"
            className="w-full"
            isDisabled
            aria-describedby="library-actions-help"
          >
            <RefreshCw aria-hidden="true" />
            全量同步
          </Button>
          <Button
            variant="secondary"
            className="w-full"
            isDisabled
            aria-describedby="library-actions-help"
          >
            <Zap aria-hidden="true" />
            增量同步
          </Button>
          <Button
            variant="secondary"
            className="w-full"
            isDisabled
            aria-describedby="library-actions-help"
          >
            <Play aria-hidden="true" />
            执行预览
          </Button>
          <DetailLink
            to={`/libraries/${item.id}/edit`}
            className="flex min-h-10 w-full items-center justify-center gap-2 rounded-xl bg-surface-secondary px-3 font-semibold text-foreground no-underline"
          >
            <Pencil aria-hidden="true" className="size-5 text-accent" />
            编辑配置
          </DetailLink>
        </div>
      </div>
      <p id="library-actions-help" className="mt-2 text-xs text-muted">
        同步和执行预览尚未开放；以下演示数据仅供界面预览。
      </p>
      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        <DetailCard title="库概览" icon={Film}>
          <div className="flex items-center gap-4 py-2">
            <span className="flex size-20 shrink-0 items-center justify-center rounded-3xl bg-accent-soft text-accent">
              <Film aria-hidden="true" className="size-11" />
            </span>
            <div className="min-w-0">
              <h3 className="mb-3 wrap-anywhere text-2xl font-semibold">
                {item.name}
              </h3>
              <div className="flex flex-wrap gap-2">
                <Chip color="accent" variant="soft" size="sm">
                  {typeLabels[item.media_type]}
                </Chip>
                <Chip
                  color={item.enabled ? "success" : "warning"}
                  variant="soft"
                  size="sm"
                >
                  <span
                    aria-hidden="true"
                    className="size-2 rounded-full bg-current"
                  />
                  {item.enabled ? "已启用" : "已暂停"}
                </Chip>
              </div>
            </div>
          </div>
        </DetailCard>
        <DetailCard title="当前状态" icon={Activity}>
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <Chip
              color={system.data ? "success" : "default"}
              variant="soft"
              size="sm"
            >
              <span
                aria-hidden="true"
                className="size-2 rounded-full bg-current"
              />
              {system.data
                ? "服务可访问"
                : system.isPending
                  ? "正在获取系统信息"
                  : "系统信息不可用"}
            </Chip>
            <Button
              size="sm"
              variant="ghost"
              isDisabled={pending}
              onPress={onToggle}
            >
              {item.enabled ? (
                <Pause aria-hidden="true" className="size-4" />
              ) : (
                <Play aria-hidden="true" className="size-4" />
              )}
              {item.enabled ? "暂停媒体库" : "启用媒体库"}
            </Button>
          </div>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
            <dt className="flex items-center gap-2 text-muted">
              <Timer aria-hidden="true" className="size-4" />
              运行时长
            </dt>
            <dd className="text-right font-medium">
              {system.data ? formatUptime(system.data.uptime_seconds) : "—"}
            </dd>
            <dt className="flex items-center gap-2 text-muted">
              <BadgeInfo aria-hidden="true" className="size-4" />
              版本信息
            </dt>
            <dd className="wrap-anywhere text-right font-medium">
              {system.data?.version ?? "—"}
            </dd>
          </dl>
          {system.isError && (
            <Alert status="warning" className="mt-3">
              <Alert.Content>
                <Alert.Description>
                  无法获取系统信息。
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => void system.refetch()}
                  >
                    重试
                  </Button>
                </Alert.Description>
              </Alert.Content>
            </Alert>
          )}
        </DetailCard>
        <DetailCard title="绑定目录" icon={Folder}>
          <DetailLink
            to={`/cloud-drive?${new URLSearchParams({ directory_id: item.directory_id })}`}
            className="flex w-full items-center gap-3 rounded-xl bg-surface-secondary p-4 text-foreground no-underline"
          >
            <Folder aria-hidden="true" className="size-6 shrink-0 text-muted" />
            <span className="min-w-0 flex-1 wrap-anywhere font-medium">
              {item.directory_path}
            </span>
            <ChevronRight aria-hidden="true" className="size-5 shrink-0" />
          </DetailLink>
        </DetailCard>
        <DetailCard title="输出设置摘要" icon={Settings2}>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-3 text-sm">
            <dt className="text-muted">输出目录</dt>
            <dd className="wrap-anywhere font-medium">{item.output_path}</dd>
            <dt className="text-muted">文件编码</dt>
            <dd className="font-medium">{item.file_encoding}</dd>
            <dt className="text-muted">生成 NFO</dt>
            <dd className="font-medium">
              {item.generate_nfo ? "已启用" : "已关闭"}
            </dd>
          </dl>
        </DetailCard>
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-[5fr_3fr]">
        <DetailCard title="STRM 统计" icon={Link2} demonstration>
          <dl className="grid grid-cols-2 gap-y-5 py-3 sm:grid-cols-4">
            {statistics.map(({ label, value, icon: Icon, color }) => (
              <div
                key={label}
                className="min-w-0 border-divider px-3 even:border-l sm:border-l sm:first:border-l-0"
              >
                <dd className="flex items-center gap-2 text-lg font-bold tabular-nums xl:text-2xl">
                  <Icon
                    aria-hidden="true"
                    className={`size-4 shrink-0 ${color}`}
                  />
                  {value.toLocaleString("en-US")}
                </dd>
                <dt className="mt-1 text-sm text-muted">{label}</dt>
              </div>
            ))}
          </dl>
        </DetailCard>
        <DetailCard title="最近同步时间" icon={Clock3} demonstration>
          <div className="mb-4 flex flex-wrap items-center gap-2 text-lg font-semibold tabular-nums">
            <LocalTime at={demo.latestSync.at} />
            <CheckCircle2
              aria-label="同步成功"
              className="size-5 text-success"
            />
          </div>
          <dl className="grid grid-cols-2 gap-2 text-sm">
            <dt className="text-muted">同步类型</dt>
            <dd className="text-right font-medium">{demo.latestSync.type}</dd>
            <dt className="text-muted">耗时</dt>
            <dd className="text-right font-medium">
              {demo.latestSync.duration}
            </dd>
          </dl>
        </DetailCard>
      </div>
      <div className="mt-4">
        <DetailCard title="执行记录摘要" icon={ListChecks} demonstration>
          <ol className="py-1">
            {demo.records.map((record) => (
              <li
                key={record.id}
                className="relative ml-2 border-l border-accent-soft py-3 pl-6 last:border-transparent"
              >
                <span
                  aria-hidden="true"
                  className="absolute top-5 -left-1 size-2 rounded-full border-2 border-accent bg-surface"
                />
                <div className="flex flex-wrap items-center gap-3 text-sm">
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
                    {record.type === "执行预览" ? (
                      <ScanLine aria-hidden="true" className="size-4" />
                    ) : (
                      <FileClock aria-hidden="true" className="size-4" />
                    )}
                  </span>
                  <span className="min-w-28 text-muted tabular-nums">
                    <LocalTime at={record.at} compact />
                  </span>
                  <Chip size="sm" variant="soft" color="accent">
                    {record.type}
                  </Chip>
                  <p className="min-w-0 basis-full wrap-anywhere pl-11 text-foreground xl:basis-auto xl:flex-1 xl:pl-0">
                    {record.summary}
                  </p>
                  <Chip size="sm" variant="soft" color="success">
                    {record.result}
                  </Chip>
                </div>
              </li>
            ))}
          </ol>
        </DetailCard>
      </div>
    </>
  );
}
