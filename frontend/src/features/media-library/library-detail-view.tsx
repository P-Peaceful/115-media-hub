import { Alert, Button, Card, Chip, Link as HeroLink } from "@heroui/react";
import { fromDate, toCalendarDate, today } from "@internationalized/date";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  ArrowLeft,
  ChevronRight,
  CircleCheck,
  CircleX,
  Clock3,
  Film,
  Folder,
  Link2,
  ListChecks,
  Pencil,
  RefreshCw,
  Settings2,
  Timer,
  Zap,
} from "lucide-react";
import { type ReactNode, useId } from "react";
import { Link as RouterLink, useNavigate } from "react-router";

import { PageBanner } from "@/components/page-banner";
import {
  formatUptime,
  useSystemStatus,
} from "@/features/dashboard/system-status";
import {
  strmLibraryPath,
  useStrmFiles,
} from "@/features/strm-management/queries";
import { StrmError } from "@/features/strm-management/strm-ui";
import type { Task } from "@/features/task-center/api";
import { taskTypes } from "@/features/task-center/task-kinds";
import {
  defaultTimezone,
  formatCount,
  formatDate,
  formatDuration,
  StatusChip,
} from "@/features/task-center/task-ui";

import type { Library } from "./api";
import {
  existingFilePolicyLabels,
  languageLabels,
  linkFormatLabels,
  linkTypeLabels,
  mediaTypeLabels,
  scrapeSourceLabels,
} from "./library-configuration-labels";
import { libraryDetailPath } from "./library-detail-context";
import {
  librarySyncHistoryPath,
  SyncHistoryState,
  SyncTaskTime,
  useLibrarySyncHistory,
} from "./library-sync-history";

const detailActions = [
  { path: "full-sync", label: "全量同步", icon: RefreshCw },
  { path: "incremental-sync", label: "增量同步", icon: Zap },
  { path: "edit", label: "编辑配置", icon: Pencil },
];

function DetailCard({
  title,
  icon: Icon,
  action,
  children,
}: {
  title: string;
  icon: LucideIcon;
  action?: ReactNode;
  children: ReactNode;
}) {
  const titleId = useId();
  return (
    <Card
      render={(props) => <section {...props} />}
      aria-labelledby={titleId}
      className="h-full min-w-0"
    >
      <Card.Header>
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          <Card.Title
            render={(props) => <h2 {...props}>{props.children}</h2>}
            id={titleId}
          >
            <span className="flex items-center gap-3 text-lg font-semibold">
              {action ? (
                <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-accent-soft text-accent">
                  <Icon aria-hidden="true" className="size-5" />
                </span>
              ) : (
                <Icon aria-hidden="true" className="size-6 text-accent" />
              )}
              {title}
            </span>
          </Card.Title>
          {action && <div className="text-sm">{action}</div>}
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

function ConfigurationFields({
  fields,
}: {
  fields: readonly (readonly [label: string, value: ReactNode])[];
}) {
  return (
    <dl className="divide-y divide-divider text-sm">
      {fields.map(([label, value]) => (
        <div
          key={label}
          className="grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] items-center gap-x-3 py-2 first:pt-0 last:pb-0"
        >
          <dt className="wrap-anywhere text-muted">{label}</dt>
          <dd className="min-w-0 wrap-anywhere font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function settingStatus(enabled: boolean) {
  return enabled ? "已启用" : "已关闭";
}

function StrmStatistics({ libraryId }: { libraryId: string }) {
  const strm = useStrmFiles({ library_id: libraryId, limit: 1 });
  const registeredCount = strm.data?.total;
  const metrics = [
    { label: "总数量", count: registeredCount },
    {
      label: "已生成",
      count: registeredCount,
      icon: CircleCheck,
      color: "text-success",
    },
    { label: "待处理", icon: Clock3, color: "text-warning-soft-foreground" },
    { label: "失败", icon: CircleX, color: "text-danger" },
  ];

  return (
    <DetailCard
      title="STRM 统计"
      icon={Link2}
      action={
        <DetailLink to={strmLibraryPath(libraryId)}>
          查看 STRM <ChevronRight aria-hidden="true" className="size-4" />
        </DetailLink>
      }
    >
      <div className="@container">
        <dl className="grid grid-cols-2 gap-y-3 py-2 @min-[25rem]:grid-cols-4">
          {metrics.map(({ label, count, icon: Icon, color }) => (
            <div
              key={label}
              className="flex min-w-0 flex-col-reverse items-center gap-1 border-divider px-1 odd:border-e @min-[25rem]:border-e @min-[25rem]:first:items-start @min-[25rem]:last:border-e-0"
            >
              <dt className="text-sm text-muted">{label}</dt>
              <dd className="flex flex-wrap items-center justify-center gap-x-1.5 text-xl leading-tight font-semibold break-all tabular-nums">
                {Icon && (
                  <Icon
                    aria-hidden="true"
                    className={`size-4 shrink-0 ${color}`}
                  />
                )}
                <span>{formatCount(count)}</span>
              </dd>
            </div>
          ))}
        </dl>
      </div>
      {strm.isPending && (
        <p className="text-sm text-muted" role="status">
          正在加载 STRM 统计…
        </p>
      )}
      {strm.isError && (
        <StrmError error={strm.error} onRetry={() => void strm.refetch()} />
      )}
    </DetailCard>
  );
}

const syncHistoryTime = new Intl.DateTimeFormat("zh-CN", {
  timeZone: defaultTimezone,
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});
const syncHistoryDate = new Intl.DateTimeFormat("zh-CN", {
  timeZone: defaultTimezone,
  month: "2-digit",
  day: "2-digit",
});

function formatSyncHistoryTime(value: string) {
  const instant = new Date(value);
  const localDate = toCalendarDate(fromDate(instant, defaultTimezone));
  const currentDate = today(defaultTimezone);
  const time = syncHistoryTime.format(instant);
  if (localDate.compare(currentDate) === 0) return `今天 ${time}`;
  if (localDate.compare(currentDate.subtract({ days: 1 })) === 0)
    return `昨天 ${time}`;
  return `${syncHistoryDate.format(instant).replace("/", "-")} ${time}`;
}

function formatSyncHistoryDuration(milliseconds: number) {
  const totalSeconds = Math.floor(milliseconds / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor(totalSeconds / 60) % 60;
  const seconds = totalSeconds % 60;
  const parts = [];
  if (hours > 0) parts.push(`${hours} 小时`);
  if (minutes > 0) parts.push(`${minutes} 分`);
  parts.push(`${seconds} 秒`);
  return parts.join(" ");
}

function SyncHistoryRow({ task }: { task: Task }) {
  const type = taskTypes[task.type];
  const Icon = task.type === "strm" ? RefreshCw : type.icon;
  const time = task.started_at ?? task.created_at;
  const counts = `生成 ${formatCount(task.succeeded_items)} 个 STRM，跳过 ${formatCount(task.skipped_items)} 个，失败 ${formatCount(task.failed_items)} 个`;
  const duration =
    task.duration_ms === null
      ? ""
      : `，耗时 ${formatSyncHistoryDuration(task.duration_ms)}`;

  return (
    <li className="group relative pl-5">
      <span
        aria-hidden="true"
        className="absolute top-7 -bottom-7 left-1.5 border-s border-accent/35 group-last:hidden"
      />
      <span
        aria-hidden="true"
        className="absolute top-6 left-0.5 size-2.5 rounded-full border-2 border-accent bg-surface"
      />
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 py-2.5 text-sm @min-[40rem]:grid-cols-[auto_6.5rem_auto_minmax(0,1fr)_auto]">
        <span className="col-start-1 row-start-1 grid size-9 place-items-center rounded-full bg-accent-soft text-accent">
          <Icon aria-hidden="true" className="size-4" />
        </span>
        <time
          className="col-start-2 row-start-1 text-muted tabular-nums"
          dateTime={time}
          title={formatDate(time)}
        >
          {formatSyncHistoryTime(time)}
        </time>
        <div className="col-start-3 row-start-1">
          <Chip color={type.color} variant="soft" size="sm">
            <Chip.Label>
              {task.type === "strm" ? "全量同步" : type.label}
            </Chip.Label>
          </Chip>
        </div>
        <div className="col-span-2 col-start-2 row-start-2 min-w-0 wrap-anywhere @min-[40rem]:col-span-1 @min-[40rem]:col-start-4 @min-[40rem]:row-start-1">
          <DetailLink to={`/tasks/${encodeURIComponent(task.id)}`}>
            {counts}
            {duration}
          </DetailLink>
        </div>
        <div className="col-start-2 row-start-3 @min-[40rem]:col-start-5 @min-[40rem]:row-start-1 @min-[40rem]:justify-self-end">
          {task.outcome === "success" && task.status === "completed" ? (
            <Chip color="success" variant="soft" size="sm">
              <Chip.Label>成功</Chip.Label>
            </Chip>
          ) : (
            <StatusChip task={task} />
          )}
        </div>
      </div>
    </li>
  );
}

export function LibraryDetailView({
  item,
}: {
  item: Library;
  onToggle: () => void;
  pending: boolean;
}) {
  const system = useSystemStatus();
  const navigate = useNavigate();
  const detailPath = libraryDetailPath(item.id);
  const history = useLibrarySyncHistory(item.id);
  const records = history.data?.items.slice(0, 4) ?? [];
  const latest = records[0];
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
        description={`${mediaTypeLabels[item.media_type]}类型媒体库，绑定 115 网盘目录，管理 STRM 输出配置。`}
      />
      <div className="mt-4 rounded-2xl bg-surface p-2 shadow-sm">
        <div className="grid grid-cols-1 gap-3 min-[380px]:grid-cols-2 lg:grid-cols-3">
          {detailActions.map(({ path, label, icon: Icon }) => (
            <Button
              key={path}
              fullWidth
              variant="secondary"
              onPress={() =>
                void navigate(
                  `${detailPath}/${path}`,
                  path === "edit"
                    ? { state: { fromLibraryPage: true } }
                    : undefined,
                )
              }
            >
              <Icon aria-hidden="true" className="size-5 shrink-0" />
              {label}
            </Button>
          ))}
        </div>
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        <DetailCard title="库概览" icon={Film}>
          <div className="@container">
            <div className="grid items-center gap-4 py-2 @min-[26rem]:grid-cols-2 @min-[26rem]:gap-6">
              <div className="flex min-w-0 items-center gap-4">
                <span className="flex size-20 shrink-0 items-center justify-center rounded-3xl bg-accent-soft text-accent">
                  <Film aria-hidden="true" className="size-11" />
                </span>
                <div className="min-w-0">
                  <h3 className="mb-3 wrap-anywhere text-2xl font-semibold">
                    {item.name}
                  </h3>
                  <div className="flex flex-wrap gap-2">
                    <Chip color="accent" variant="soft" size="sm">
                      {mediaTypeLabels[item.media_type]}
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
              <div className="min-w-0">
                <ConfigurationFields
                  fields={[
                    ["语言偏好", languageLabels[item.language_preference]],
                    ["刮削源", scrapeSourceLabels[item.scrape_source]],
                  ]}
                />
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
          </div>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
            <dt className="flex items-center gap-2 text-muted">
              <Timer aria-hidden="true" className="size-4" />
              运行时长
            </dt>
            <dd className="text-right font-medium">
              {system.data ? formatUptime(system.data.uptime_seconds) : "—"}
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
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-3">
        <DetailCard title="绑定目录" icon={Folder}>
          <dl className="space-y-2 text-sm">
            <dt className="text-muted">媒体库目录</dt>
            <dd>
              <DetailLink
                to={`/cloud-drive?${new URLSearchParams({ directory_id: item.directory_id })}`}
                className="flex w-full items-center gap-3 rounded-xl bg-surface-secondary p-4 text-foreground no-underline"
              >
                <Folder
                  aria-hidden="true"
                  className="size-6 shrink-0 text-muted"
                />
                <span className="min-w-0 flex-1 wrap-anywhere font-medium">
                  {item.directory_path}
                </span>
                <ChevronRight aria-hidden="true" className="size-5 shrink-0" />
              </DetailLink>
            </dd>
          </dl>
          <div className="mt-4">
            <ConfigurationFields
              fields={[
                ["排除目录", item.exclude_directory_path || "未设置"],
                [
                  "包含子目录（递归扫描）",
                  settingStatus(item.include_subdirectories),
                ],
                [
                  "跟随网盘目录变更",
                  settingStatus(item.follow_directory_changes),
                ],
              ]}
            />
          </div>
        </DetailCard>
        <DetailCard title="输出设置" icon={Settings2}>
          <ConfigurationFields
            fields={[
              ["输出目录", item.output_path],
              ["STRM 文件", settingStatus(item.generate_strm)],
              ["NFO 信息", settingStatus(item.generate_nfo)],
              ["海报与封面", settingStatus(item.generate_poster_cover)],
              [
                "STRM 已存在时",
                existingFilePolicyLabels[item.strm_existing_file_policy],
              ],
              ["文件编码", item.file_encoding],
            ]}
          />
        </DetailCard>
        <DetailCard title="STRM 链接配置" icon={Link2}>
          <ConfigurationFields
            fields={[
              ["链接类型", linkTypeLabels[item.link_type]],
              ["STRM 播放地址", item.strm_direct_domain || "未设置"],
              ["链接格式", linkFormatLabels[item.strm_link_format]],
            ]}
          />
        </DetailCard>
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-[5fr_3fr]">
        <StrmStatistics libraryId={item.id} />
        <DetailCard title="最近同步时间" icon={Clock3}>
          <SyncHistoryState history={history} />
          {latest && (
            <>
              <div className="mb-4 flex flex-wrap items-center gap-2 text-lg font-semibold">
                <SyncTaskTime task={latest} />
                <StatusChip task={latest} />
              </div>
              <dl className="grid grid-cols-2 gap-2 text-sm">
                <dt className="text-muted">同步类型</dt>
                <dd className="text-right font-medium">全量同步（STRM）</dd>
                <dt className="text-muted">耗时</dt>
                <dd className="text-right font-medium">
                  {formatDuration(latest.duration_ms)}
                </dd>
              </dl>
            </>
          )}
        </DetailCard>
      </div>
      <div className="mt-4">
        <DetailCard
          title="执行记录摘要"
          icon={ListChecks}
          action={
            <DetailLink to={librarySyncHistoryPath(item.id)}>
              查看全部 <ChevronRight aria-hidden="true" className="size-4" />
            </DetailLink>
          }
        >
          <SyncHistoryState history={history} />
          {records.length > 0 && (
            <div className="@container">
              <ol className="py-1">
                {records.map((record) => (
                  <SyncHistoryRow key={record.id} task={record} />
                ))}
              </ol>
            </div>
          )}
        </DetailCard>
      </div>
    </>
  );
}
