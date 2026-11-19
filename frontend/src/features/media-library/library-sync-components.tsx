import { Button, Card, Chip } from "@heroui/react";
import type { LucideIcon } from "lucide-react";
import { ArrowLeft, CheckCircle2, Play } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Link } from "react-router";

import folderArtwork from "@/assets/media-folder.svg";

import {
  libraryDetailPath,
  useLibraryDetailContext,
} from "./library-detail-context";
import {
  type ChangeKind,
  changeKinds,
  changeLabels,
  formatSyncCount,
} from "./library-sync-demo";

const changeColors = {
  added: "success",
  updated: "accent",
  skipped: "default",
  deleted: "danger",
} as const;

export function SyncPageHeader({
  title,
  description,
  icon: Icon,
  backToFullSync = false,
}: {
  title: string;
  description: string;
  icon: LucideIcon;
  backToFullSync?: boolean;
}) {
  const { item } = useLibraryDetailContext();
  return (
    <header className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <Link
          className="inline-flex items-center gap-2 font-medium text-accent hover:underline"
          to={`${libraryDetailPath(item.id)}${backToFullSync ? "/full-sync" : ""}`}
        >
          <ArrowLeft aria-hidden="true" className="size-4" />
          {backToFullSync ? "返回全量同步" : "返回媒体库详情"}
        </Link>
        <span className="min-w-0 wrap-anywhere text-muted">{item.name}</span>
      </div>
      <div className="page-banner flex min-h-24 items-center gap-4 rounded-2xl px-4 sm:px-6">
        <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-surface/75 text-accent sm:size-13">
          <Icon aria-hidden="true" className="size-6 sm:size-7" />
        </span>
        <div className="min-w-0 flex-1 space-y-1.5 py-5">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h1
              data-page-banner-title
              className="text-2xl font-bold tracking-tight"
            >
              {title}
            </h1>
            <Chip size="sm" variant="soft" color="accent">
              演示数据
            </Chip>
          </div>
          <p className="text-sm text-foreground">{description}</p>
        </div>
        <img
          className="pointer-events-none hidden h-24 w-36 shrink-0 object-contain sm:block"
          src={folderArtwork}
          alt=""
        />
      </div>
    </header>
  );
}

export function SyncSection({
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
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="min-w-0">
      <Card className="h-full border border-border shadow-sm">
        <Card.Header>
          <div className="flex w-full flex-wrap items-center justify-between gap-x-3 gap-y-2">
            <h2
              id={headingId}
              className="flex items-center gap-2.5 font-semibold"
            >
              <Icon
                aria-hidden="true"
                className="size-5 shrink-0 text-accent"
              />
              {title}
            </h2>
            {action}
          </div>
        </Card.Header>
        <Card.Content>{children}</Card.Content>
      </Card>
    </section>
  );
}

export function ChangeChip({ kind }: { kind: ChangeKind }) {
  return (
    <Chip color={changeColors[kind]} size="sm" variant="soft">
      {changeLabels[kind]}
    </Chip>
  );
}

export function ResultChip({ result }: { result: "成功" | "失败" }) {
  return (
    <Chip
      color={result === "成功" ? "success" : "danger"}
      size="sm"
      variant="soft"
    >
      <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
      {result}
    </Chip>
  );
}

export function LibraryLocalTime({
  at,
  compact = false,
}: {
  at: string;
  compact?: boolean;
}) {
  return (
    <time dateTime={at} className="whitespace-nowrap tabular-nums">
      {new Date(at)
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

export function SyncCounts({ counts }: { counts: Record<ChangeKind, number> }) {
  return (
    <span className="inline-flex flex-wrap gap-x-3 gap-y-1 tabular-nums">
      {changeKinds.map((kind) => (
        <span key={kind} className="whitespace-nowrap">
          {changeLabels[kind]} {formatSyncCount(counts[kind])}
        </span>
      ))}
    </span>
  );
}

export function DemoExecutionAction({
  label,
  summary,
  secondaryAction,
}: {
  label: string;
  summary?: ReactNode;
  secondaryAction?: ReactNode;
}) {
  const [completed, setCompleted] = useState(false);
  return (
    <Card>
      <Card.Content>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0 flex-1 basis-full text-sm sm:basis-60">
            {summary}
            <div role="status" className="text-success">
              {completed && (
                <p className="flex items-center gap-2">
                  <CheckCircle2
                    aria-hidden="true"
                    className="size-4 shrink-0"
                  />
                  演示执行已完成。
                </p>
              )}
            </div>
          </div>
          <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
            {secondaryAction}
            <Button onPress={() => setCompleted(true)}>
              <Play aria-hidden="true" className="size-4" />
              {label}
            </Button>
          </div>
        </div>
      </Card.Content>
    </Card>
  );
}

export function SyncRecordDetails({
  title,
  closeLabel,
  onClose,
  children,
}: {
  title: string;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="mb-3 rounded-xl border border-border bg-surface-secondary p-4"
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3
          id={headingId}
          className="font-semibold"
          tabIndex={-1}
          ref={focusRecordHeading}
        >
          {title}
        </h3>
        <Button size="sm" variant="ghost" onPress={onClose}>
          {closeLabel}
        </Button>
      </div>
      {children}
    </section>
  );
}

function focusRecordHeading(heading: HTMLHeadingElement | null) {
  heading?.focus();
}
