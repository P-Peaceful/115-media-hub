import {
  Alert,
  AlertDialog,
  Button,
  Card,
  Chip,
  Description,
  EmptyState as HeroEmptyState,
  Label,
  Link,
  ListBox,
  Pagination,
  ProgressBar,
  Select,
  Spinner,
} from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { ArrowLeft } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";
import { useId } from "react";

import { ApiError } from "@/lib/api/errors";

import type { Task } from "./api";
import type { TaskType } from "./task-kinds";
import { taskStatuses, taskTypes } from "./task-kinds";

export const taskPageClass =
  "mx-auto grid w-full min-w-0 max-w-[1600px] gap-4 px-3 pt-4 pb-8 min-[701px]:px-6 min-[701px]:pt-6 min-[701px]:pb-12";
export const headingClass = "flex flex-wrap items-center gap-3";
export const toolbarClass = "flex min-w-0 flex-wrap items-center gap-2";
export const titleClass = "wrap-anywhere text-2xl font-semibold";
export const recordLabelClass = "block max-w-52 truncate";
export const emptyMetric = "—";
export const defaultTimezone = "Asia/Shanghai";
const dateOptions = {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
} satisfies Intl.DateTimeFormatOptions;
const dateFormat = new Intl.DateTimeFormat("zh-CN", {
  ...dateOptions,
  timeZone: defaultTimezone,
});
const numberFormat = new Intl.NumberFormat("zh-CN");
const taskErrorMessages: Record<string, string> = {
  task_type_unavailable: "此任务类型暂不可用。",
  task_state_conflict: "当前状态已变化，请刷新后重试。",
  task_action_unavailable: "当前任务无法执行此操作，请刷新后重试。",
  task_not_found: "找不到该任务。",
  schedule_not_found: "找不到该定时计划。",
  library_not_found: "找不到目标媒体库。",
  library_unavailable: "目标媒体库暂不可用。",
  not_found: "找不到相关记录。",
  conflict: "记录已变化，请刷新后重试。",
};

export function formatCount(count: number | undefined | null) {
  return count == null ? emptyMetric : numberFormat.format(count);
}
export function formatDate(at: string | null, timezone = defaultTimezone) {
  if (at === null) return emptyMetric;
  const instant = new Date(at);
  if (timezone === defaultTimezone) return dateFormat.format(instant);
  try {
    return new Intl.DateTimeFormat("zh-CN", {
      ...dateOptions,
      timeZone: timezone,
    }).format(instant);
  } catch {
    return dateFormat.format(instant);
  }
}
export function formatDuration(ms: number | null) {
  if (ms === null) return emptyMetric;
  const seconds = Math.floor(ms / 1000);
  return [
    Math.floor(seconds / 3600),
    Math.floor(seconds / 60) % 60,
    seconds % 60,
  ]
    .map((value) => String(value).padStart(2, "0"))
    .join(":");
}
export function formatSpeed(bytes: number | null) {
  if (bytes === null) return emptyMetric;
  if (bytes >= 2 ** 20) return `${(bytes / 2 ** 20).toFixed(1)} MB/s`;
  if (bytes >= 2 ** 10) return `${(bytes / 2 ** 10).toFixed(1)} KB/s`;
  return `${bytes.toFixed(0)} B/s`;
}

export function Panel({
  title,
  icon: Icon,
  actions,
  children,
  className = "",
}: {
  title: ReactNode;
  icon: typeof ArrowLeft;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  const id = useId();
  return (
    <section className={`grid min-w-0 ${className}`} aria-labelledby={id}>
      <Card className="min-w-0">
        <Card.Header>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Card.Title
              render={(props) => <h2 {...props}>{props.children}</h2>}
              id={id}
            >
              <span className="flex items-center gap-2">
                <Icon aria-hidden="true" className="size-4 shrink-0" />
                {title}
              </span>
            </Card.Title>
            {actions}
          </div>
        </Card.Header>
        <Card.Content>
          <div className="grid min-w-0 gap-3">{children}</div>
        </Card.Content>
      </Card>
    </section>
  );
}

export function TaskError({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  if (!error) return null;
  return (
    <Alert status="danger">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Description className="wrap-anywhere">
          {error instanceof ApiError
            ? (taskErrorMessages[error.code] ?? error.message)
            : "操作未能完成，请稍后重试。"}
          {error instanceof ApiError && error.requestId
            ? `（请求 ID ${error.requestId}）`
            : ""}
        </Alert.Description>
        {retry && (
          <div>
            <Button size="sm" variant="secondary" onPress={retry}>
              重新加载
            </Button>
          </div>
        )}
      </Alert.Content>
    </Alert>
  );
}

export function TaskLoading({
  children = "正在加载…",
}: {
  children?: ReactNode;
}) {
  return (
    <div
      className="grid min-h-36 place-content-center justify-items-center gap-2 text-sm text-muted"
      role="status"
    >
      <Spinner size="sm" aria-hidden="true" />
      {children}
    </div>
  );
}

export function EmptyState({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof ArrowLeft;
  title: string;
  children?: ReactNode;
}) {
  return (
    <HeroEmptyState>
      <div className="grid min-h-36 place-content-center justify-items-center gap-2 text-center">
        <Icon className="size-6" aria-hidden="true" />
        <strong className="text-foreground">{title}</strong>
        {children && <p>{children}</p>}
      </div>
    </HeroEmptyState>
  );
}

export type SelectOption = { id: string; label: string };
export function TaskSelect({
  label,
  value,
  options,
  onChange,
  disabled = false,
  showLabel = false,
  description,
}: {
  label: string;
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  showLabel?: boolean;
  description?: ReactNode;
}) {
  return (
    <Select
      {...(showLabel ? {} : { "aria-label": label })}
      placeholder={label}
      variant="secondary"
      value={value || null}
      onChange={(value) => {
        if (typeof value === "string") onChange(value);
      }}
      isDisabled={disabled}
    >
      {showLabel && <Label>{label}</Label>}
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox items={options}>
          {(option) => (
            <ListBox.Item id={option.id} textValue={option.label}>
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          )}
        </ListBox>
      </Select.Popover>
      {description && <Description>{description}</Description>}
    </Select>
  );
}

export function TypeChip({ type }: { type: TaskType }) {
  const kind = taskTypes[type];
  return (
    <Chip size="sm" variant="soft" color={kind.color}>
      <Chip.Label>{kind.label}</Chip.Label>
    </Chip>
  );
}
export function StatusChip({ task }: { task: Task }) {
  const status = taskStatuses[task.status];
  return (
    <Chip
      size="sm"
      variant="soft"
      color={task.outcome === "partial_failure" ? "warning" : status.color}
    >
      <Chip.Label>
        {task.outcome === "partial_failure"
          ? "已完成（部分失败）"
          : status.label}
      </Chip.Label>
    </Chip>
  );
}

export function TaskProgress({ value }: { value: number | null }) {
  if (value === null) return <span className="text-muted">{emptyMetric}</span>;
  return (
    <ProgressBar
      aria-label="执行进度"
      value={value}
      formatOptions={{
        style: "percent",
        minimumFractionDigits: 1,
        maximumFractionDigits: 1,
      }}
    >
      <ProgressBar.Track>
        <ProgressBar.Fill />
      </ProgressBar.Track>
      <ProgressBar.Output />
    </ProgressBar>
  );
}

export function TaskPagination({
  cursor,
  next,
  onChange,
}: {
  cursor: string;
  next?: string | null | undefined;
  onChange: (cursor: string) => void;
}) {
  if (!cursor && !next) return null;
  return (
    <div className="flex justify-end">
      <Pagination aria-label="分页" size="sm">
        <Pagination.Content>
          {cursor && (
            <Pagination.Item>
              <Pagination.Previous onPress={() => onChange("")}>
                <Pagination.PreviousIcon /> 返回第一页
              </Pagination.Previous>
            </Pagination.Item>
          )}
          {next && (
            <Pagination.Item>
              <Pagination.Next onPress={() => onChange(next)}>
                下一页 <Pagination.NextIcon />
              </Pagination.Next>
            </Pagination.Item>
          )}
        </Pagination.Content>
      </Pagination>
    </div>
  );
}

export function TaskButtonLink({
  href,
  children,
  variant = "secondary",
  isIconOnly = false,
  "aria-label": label,
}: Pick<
  ComponentProps<typeof Button>,
  "variant" | "isIconOnly" | "aria-label"
> & {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      {...(label ? { "aria-label": label } : {})}
      className={buttonVariants({ variant, size: "sm", isIconOnly })}
    >
      {children}
    </Link>
  );
}

export function TaskBackLink({ href, label }: { href: string; label: string }) {
  return (
    <TaskButtonLink href={href} aria-label={label} isIconOnly>
      <ArrowLeft aria-hidden="true" />
    </TaskButtonLink>
  );
}

export function ConfirmAction({
  open,
  onOpenChange,
  title,
  body,
  label,
  pending,
  error,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  body: string;
  label: string;
  pending: boolean;
  error: unknown;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog.Backdrop
      isOpen={open}
      isDismissable={false}
      isKeyboardDismissDisabled={pending}
      onOpenChange={(next) => {
        if (!pending) onOpenChange(next);
      }}
    >
      <AlertDialog.Container>
        <AlertDialog.Dialog>
          <AlertDialog.Header>
            <AlertDialog.Heading>{title}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p className="wrap-anywhere">{body}</p>
            <TaskError error={error} />
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button
              variant="secondary"
              isDisabled={pending}
              onPress={() => onOpenChange(false)}
            >
              返回
            </Button>
            <Button variant="danger" isPending={pending} onPress={onConfirm}>
              {label}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  );
}
