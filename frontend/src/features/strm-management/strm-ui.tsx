import {
  Alert,
  Card,
  Chip,
  EmptyState,
  Label,
  ListBox,
  SearchField,
  Select,
} from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import type { LucideIcon } from "lucide-react";
import { ArrowLeft, FileX2 } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

type Tone = "success" | "danger" | "warning" | "accent" | "neutral";
const toneClass: Record<Tone, string> = {
  accent: "bg-accent-soft text-accent-soft-foreground",
  success: "bg-success-soft text-success-soft-foreground",
  danger: "bg-danger-soft text-danger-soft-foreground",
  warning: "bg-warning-soft text-warning-soft-foreground",
  neutral: "bg-default text-default-foreground",
};

export function DemoBadge() {
  return (
    <Chip size="sm" variant="soft">
      <Chip.Label>演示数据</Chip.Label>
    </Chip>
  );
}

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
      className="whitespace-nowrap"
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
  detail,
}: {
  label: string;
  value: ReactNode;
  icon: LucideIcon;
  tone: Tone;
  detail?: ReactNode;
}) {
  return (
    <Card className="strm-card">
      <Card.Content className="strm-card-row min-h-20 items-center gap-3 px-3 py-2.5 sm:gap-4 sm:px-4">
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
          {detail}
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
        className: "bg-surface",
      })}
    >
      <ArrowLeft aria-hidden="true" className="size-4" />
    </Link>
  );
}

export function strmReturnTo(state: unknown) {
  if (typeof state !== "object" || state === null || !("returnTo" in state))
    return "/strm";
  const { returnTo } = state;
  return typeof returnTo === "string" &&
    /^\/strm(?:\/check-results)?(?:\?|$)/.test(returnTo)
    ? returnTo
    : "/strm";
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
    <Card className="strm-card">
      <Card.Header className="px-4 pt-4 pb-3">
        <Card.Title
          render={(props) => <h2 {...props}>{props.children}</h2>}
          className="flex items-center gap-2 text-base font-semibold"
        >
          <span className="grid size-7 place-items-center rounded-lg bg-accent-soft text-accent-soft-foreground">
            <Icon aria-hidden="true" className="size-4" />
          </span>
          {title}
        </Card.Title>
      </Card.Header>
      <Card.Content className="min-w-0 px-4 pt-0 pb-4">{children}</Card.Content>
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
    <EmptyState className="min-h-36 px-4 py-6 text-center">
      <div className="grid justify-items-center gap-2">
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

export function StrmFeedback({ message }: { message: string | null }) {
  return (
    message && (
      <Alert status="success" className="py-2" role="status">
        <Alert.Indicator />
        <Alert.Content>
          <Alert.Description>{message}</Alert.Description>
        </Alert.Content>
      </Alert>
    )
  );
}

export function StrmSelect({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: Readonly<Record<string, string>>;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Select
      value={value}
      onChange={(key) => {
        if (typeof key === "string") onChange(key);
      }}
      className="strm-select"
    >
      <Label className="shrink-0 text-sm font-medium text-muted">{label}</Label>
      <Select.Trigger className="min-h-8 min-w-0 flex-1">
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
      className="min-w-0"
      fullWidth
    >
      <SearchField.Group className="min-h-8">
        <SearchField.SearchIcon />
        <SearchField.Input placeholder={placeholder} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  );
}
