import "./strm-detail.css";

import { Button, Chip, Modal, Surface } from "@heroui/react";
import {
  CircleCheck,
  Copy,
  ExternalLink,
  FileText,
  Folder,
  PlayCircle,
  ShieldCheck,
  SquareCode,
} from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { useLocation, useParams } from "react-router";

import type { StrmFile } from "./strm-demo";
import {
  formatStrmDate,
  formatStrmSize,
  strmLibraries,
  strmPlaybackStatusLabels,
  strmSourceStatusLabels,
  strmStatusLabels,
} from "./strm-demo";
import { useStrmSession } from "./strm-session-context";
import {
  DemoBadge,
  PageBack,
  StatusChip,
  StrmEmpty,
  StrmPanel,
  strmReturnTo,
} from "./strm-ui";

const statusTones = {
  valid: "success",
  invalid: "danger",
  pending: "warning",
} as const;

const playbackTones = {
  playable: "success",
  unplayable: "danger",
  unchecked: "warning",
} as const;

function FileMetadataField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="grid grid-cols-[5.25rem_minmax(0,1fr)] items-center gap-2 sm:grid-cols-[7rem_minmax(0,1fr)]">
      <dt className="font-medium text-muted">{label}</dt>
      <dd className="min-w-0 font-medium">{children}</dd>
    </div>
  );
}

function FileMetadata({ file }: { file: StrmFile }) {
  const [notice, setNotice] = useState("");

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice(`${label}已复制`);
    } catch {
      setNotice(`无法复制${label}，请检查浏览器权限。`);
    }
  }

  const copyFields = [
    { label: "文件名称", value: file.name },
    { label: "本地路径", value: file.localPath },
    { label: "源文件", value: file.sourcePath },
  ];

  return (
    <StrmPanel title="基础信息" icon={FileText}>
      <dl className="space-y-2 text-sm">
        {copyFields.map(({ label, value }) => (
          <FileMetadataField key={label} label={label}>
            <div className="flex items-center gap-1">
              <span className="min-w-0 wrap-anywhere">{value}</span>
              <Button
                aria-label={`复制${label}`}
                isIconOnly
                className="shrink-0"
                size="sm"
                variant="ghost"
                onPress={() => void copy(value, label)}
              >
                <Copy aria-hidden="true" className="size-3.5" />
              </Button>
            </div>
          </FileMetadataField>
        ))}
        <FileMetadataField label="所属媒体库">
          <StatusChip tone="accent">{strmLibraries[file.library]}</StatusChip>
        </FileMetadataField>
        <FileMetadataField label="生成时间">
          <time dateTime={file.generatedAt}>
            {formatStrmDate(file.generatedAt, true)}
          </time>
        </FileMetadataField>
        <FileMetadataField label="文件大小">
          {formatStrmSize(file.sizeBytes)}
        </FileMetadataField>
        <FileMetadataField label="当前状态">
          <StatusChip tone={statusTones[file.status]}>
            {strmStatusLabels[file.status]}
          </StatusChip>
        </FileMetadataField>
      </dl>
      <p
        className={notice ? "mt-2 text-xs text-muted" : "sr-only"}
        role="status"
      >
        {notice}
      </p>
    </StrmPanel>
  );
}

function SourceLocation({ file }: { file: StrmFile }) {
  const separator = file.sourcePath.lastIndexOf("/");
  const sourceDirectory = file.sourcePath.slice(0, separator + 1);
  const sourceName = file.sourcePath.slice(separator + 1);

  return (
    <StrmPanel title="源文件定位" icon={Folder}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 gap-y-1">
          <span className="inline-flex shrink-0 items-center gap-2 font-medium">
            <Folder aria-hidden="true" className="size-4 text-muted" />
            打开源文件所在目录
          </span>
          <span className="min-w-0 wrap-anywhere text-muted">
            {sourceDirectory}
          </span>
        </div>
        <Modal>
          <Button
            aria-label="定位源文件"
            className="shrink-0"
            size="sm"
            variant="outline"
          >
            定位
            <ExternalLink aria-hidden="true" className="size-4" />
          </Button>
          <Modal.Backdrop>
            <Modal.Container>
              <Modal.Dialog>
                <Modal.CloseTrigger aria-label="关闭样例源目录" />
                <Modal.Header>
                  <Modal.Heading>样例源目录</Modal.Heading>
                </Modal.Header>
                <Modal.Body>
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <Folder
                      aria-hidden="true"
                      className="size-4 shrink-0 text-accent"
                    />
                    <span className="min-w-0 wrap-anywhere">
                      {sourceDirectory}
                    </span>
                  </p>
                  {file.sourceStatus === "linked" ? (
                    <Surface
                      className="flex items-center gap-2 rounded-xl p-3 text-sm"
                      variant="secondary"
                    >
                      <FileText
                        aria-hidden="true"
                        className="size-4 shrink-0 text-accent"
                      />
                      <span className="min-w-0 wrap-anywhere">
                        {sourceName}
                      </span>
                    </Surface>
                  ) : (
                    <p className="text-sm text-muted">
                      样例目录中没有关联的源文件。
                    </p>
                  )}
                </Modal.Body>
              </Modal.Dialog>
            </Modal.Container>
          </Modal.Backdrop>
        </Modal>
      </div>
    </StrmPanel>
  );
}

function FileStatuses({ file }: { file: StrmFile }) {
  return (
    <StrmPanel title="关联与状态" icon={SquareCode}>
      <dl className="divide-y divide-border text-sm">
        <div className="flex flex-wrap items-center justify-between gap-2 py-2.5 first:pt-0">
          <dt className="inline-flex items-center gap-2.5 font-medium">
            <CircleCheck aria-hidden="true" className="size-5 text-muted" />
            源文件关联
          </dt>
          <dd>
            <StatusChip
              tone={file.sourceStatus === "linked" ? "success" : "danger"}
            >
              {strmSourceStatusLabels[file.sourceStatus]}
            </StatusChip>
          </dd>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 py-2.5">
          <dt className="inline-flex items-center gap-2.5 font-medium">
            <PlayCircle aria-hidden="true" className="size-5 text-muted" />
            播放链接
          </dt>
          <dd>
            <StatusChip tone={playbackTones[file.playbackStatus]}>
              {strmPlaybackStatusLabels[file.playbackStatus]}
            </StatusChip>
          </dd>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 pt-2.5">
          <dt className="inline-flex items-center gap-2.5 font-medium">
            <ShieldCheck aria-hidden="true" className="size-5 text-muted" />
            有效性检查
          </dt>
          <dd className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="text-xs text-muted">
              最近检查：
              {file.lastCheckedAt ? (
                <time dateTime={file.lastCheckedAt}>
                  {formatStrmDate(file.lastCheckedAt)}
                </time>
              ) : (
                "尚未检查"
              )}
            </span>
            <StatusChip tone={statusTones[file.status]}>
              {strmStatusLabels[file.status]}
            </StatusChip>
          </dd>
        </div>
      </dl>
    </StrmPanel>
  );
}

export function StrmDetailPage() {
  const { id } = useParams();
  const navigationState: unknown = useLocation().state;
  const { files, checkRun } = useStrmSession();
  const file = files.find((item) => item.id === id);
  const returnTo = strmReturnTo(navigationState);
  const isResultsReturn = returnTo.startsWith("/strm/check-results");
  const backHref = isResultsReturn && checkRun === null ? "/strm" : returnTo;
  const backLabel =
    isResultsReturn && checkRun !== null ? "返回检查结果" : "返回 STRM 管理";

  return (
    <div className="space-y-3">
      <header className="strm-detail-heading flex flex-wrap items-center gap-x-3 gap-y-1">
        <PageBack href={backHref} label={backLabel} />
        <h1>文件详情</h1>
        <DemoBadge />
      </header>
      {file ? (
        <div className="grid items-start gap-3 lg:grid-cols-2">
          <div className="grid min-w-0 gap-3">
            <FileMetadata key={file.id} file={file} />
            <SourceLocation file={file} />
          </div>
          <div className="grid min-w-0 gap-3">
            <StrmPanel title="STRM 内容预览" icon={SquareCode}>
              <Surface
                className="min-h-28 min-w-0 rounded-xl p-3 sm:p-4"
                variant="secondary"
              >
                <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
                  <pre className="min-w-0 flex-1 font-mono text-sm leading-relaxed wrap-anywhere whitespace-pre-wrap">
                    <code>{file.content}</code>
                  </pre>
                  <Chip className="shrink-0" size="sm" variant="secondary">
                    只读
                  </Chip>
                </div>
              </Surface>
            </StrmPanel>
            <FileStatuses file={file} />
          </div>
        </div>
      ) : (
        <StrmEmpty
          title="找不到此文件"
          description="样例文件可能已在本次演示中清理。"
          href={backHref}
          label={backLabel}
        />
      )}
    </div>
  );
}
