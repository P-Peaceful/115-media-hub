import {
  Alert,
  Button,
  Card,
  Chip,
  EmptyState,
  ListBox,
  SearchField,
  Select,
  Spinner,
} from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import type { LucideIcon } from "lucide-react";
import { ArrowLeft, FileX2 } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import { ApiError } from "@/lib/api/errors";

import { maxSearchLength } from "./strm-list-query";

type Tone = "success" | "danger" | "warning" | "accent" | "neutral";
const toneClass: Record<Tone, string> = {
  accent: "bg-accent-soft text-accent-soft-foreground",
  success: "bg-success-soft text-success-soft-foreground",
  danger: "bg-danger-soft text-danger-soft-foreground",
  warning: "bg-warning-soft text-warning-soft-foreground",
  neutral: "bg-default text-default-foreground",
};

export function StatusChip({
  tone,
  children,
  dot = false,
}: {
  tone: Tone;
  children: ReactNode;
  dot?: boolean;
}) {
  return (
    <Chip
      color={tone === "neutral" ? "default" : tone}
      variant="soft"
      size="sm"
    >
      {dot && (
        <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
      )}
      <Chip.Label>{children}</Chip.Label>
    </Chip>
  );
}

export function MetricTile({
  label,
  value,
  icon: Icon,
  tone,
}: {
  label: string;
  value: ReactNode;
  icon: LucideIcon;
  tone: Tone;
}) {
  return (
    <Card>
      <Card.Content>
        <div className="flex min-w-0 items-center gap-2.5 sm:gap-4">
          <span
            className={`grid size-9 shrink-0 place-items-center rounded-full sm:size-12 ${toneClass[tone]}`}
          >
            <Icon className="size-5 sm:size-6" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <div className="text-sm font-medium text-muted">{label}</div>
            <div className="text-lg leading-tight font-bold tabular-nums sm:text-2xl">
              {value}
            </div>
          </div>
        </div>
      </Card.Content>
    </Card>
  );
}

export function PageBack({ href, label }: { href: string; label: string }) {
  return (
    <Link
      to={href}
      aria-label={label}
      className={buttonVariants({
        variant: "secondary",
        size: "sm",
        isIconOnly: true,
      })}
    >
      <ArrowLeft aria-hidden="true" className="size-4" />
    </Link>
  );
}

export function strmReturnTo(state: unknown, fallback: string) {
  if (typeof state !== "object" || state === null || !("returnTo" in state))
    return fallback;
  const { returnTo } = state;
  return typeof returnTo === "string" && /^\/strm(?:\?|$)/.test(returnTo)
    ? returnTo
    : fallback;
}

export function StrmPanel({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
}) {
  return (
    <Card>
      <Card.Header>
        <Card.Title render={(props) => <h2 {...props}>{props.children}</h2>}>
          <span className="flex items-center gap-2">
            <span className="grid size-7 place-items-center rounded-lg bg-accent-soft text-accent-soft-foreground">
              <Icon aria-hidden="true" className="size-4" />
            </span>
            {title}
          </span>
        </Card.Title>
      </Card.Header>
      <Card.Content>{children}</Card.Content>
    </Card>
  );
}

export function StrmEmpty({
  title,
  description,
  href,
  label,
}: {
  title: string;
  description: string;
  href?: string;
  label?: string;
}) {
  return (
    <EmptyState>
      <div className="grid min-h-36 justify-items-center gap-2 px-4 py-6 text-center">
        <FileX2 className="size-6 text-muted" aria-hidden="true" />
        <strong>{title}</strong>
        <p className="text-sm text-muted">{description}</p>
        {href && (
          <Link
            to={href}
            className={buttonVariants({ variant: "secondary", size: "sm" })}
          >
            {label}
          </Link>
        )}
      </div>
    </EmptyState>
  );
}

export function StrmSelect({
  label,
  options,
  value,
  onChange,
  disabledReason,
  inline = false,
}: {
  label: string;
  options: Readonly<Record<string, string>>;
  value: string;
  onChange?: (value: string) => void;
  disabledReason?: string;
  inline?: boolean;
}) {
  return (
    <div
      className={
        inline
          ? "flex min-w-0 items-center gap-2"
          : "flex min-w-0 flex-col gap-1 @min-[31rem]:flex-row @min-[31rem]:items-center @min-[31rem]:gap-2"
      }
      title={disabledReason}
    >
      <span
        aria-hidden="true"
        className="shrink-0 text-sm font-medium text-muted"
      >
        {label}
      </span>
      <div className="min-w-0 flex-1">
        <Select
          aria-label={label}
          aria-description={disabledReason}
          fullWidth
          value={value}
          isDisabled={!!disabledReason}
          onChange={(key) => {
            if (typeof key === "string") onChange?.(key);
          }}
        >
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {Object.entries(options).map(([id, text]) => (
                <ListBox.Item key={id} id={id} textValue={text}>
                  {text}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </div>
    </div>
  );
}

export function StrmSearch({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <SearchField
      aria-label="搜索 STRM 文件"
      value={value}
      onChange={onChange}
      fullWidth
    >
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input
          placeholder={placeholder}
          maxLength={maxSearchLength}
        />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  );
}

export function formatStrmDate(value: string, withSeconds = false) {
  const date = new Date(value);
  const pad = (part: number) => String(part).padStart(2, "0");
  const day = [date.getFullYear(), date.getMonth() + 1, date.getDate()]
    .map(pad)
    .join("-");
  const time = [date.getHours(), date.getMinutes()];
  if (withSeconds) time.push(date.getSeconds());
  return `${day} ${time.map(pad).join(":")}`;
}

export function formatStrmSize(bytes: number) {
  if (bytes >= 2 ** 20) return `${(bytes / 2 ** 20).toFixed(1)} MB`;
  if (bytes >= 2 ** 10) return `${(bytes / 2 ** 10).toFixed(1)} KB`;
  return `${bytes} B`;
}

export function StrmLoading({ children }: { children: ReactNode }) {
  return (
    <div
      className="flex min-h-32 items-center justify-center gap-2 text-sm text-muted"
      role="status"
    >
      <Spinner size="sm" aria-hidden="true" />
      {children}
    </div>
  );
}

export function StrmError({
  error,
  onRetry,
}: {
  error: Error;
  onRetry: () => void;
}) {
  const description =
    error instanceof ApiError
      ? error.message
      : "无法加载 STRM 数据，请稍后重试。";
  return (
    <Alert role="alert" status="danger">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Description>
          {description}
          {error instanceof ApiError && error.requestId
            ? `（请求 ID ${error.requestId}）`
            : ""}
        </Alert.Description>
        <div>
          <Button size="sm" variant="secondary" onPress={onRetry}>
            重新加载
          </Button>
        </div>
      </Alert.Content>
    </Alert>
  );
}
