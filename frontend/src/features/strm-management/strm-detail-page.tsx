import { Button, Chip, Surface } from "@heroui/react";
import {
  CircleCheck,
  Copy,
  FileText,
  Folder,
  PlayCircle,
  ShieldCheck,
  SquareCode,
} from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { useLocation, useParams } from "react-router";

import { ApiError } from "@/lib/api/errors";

import type { StrmDetail } from "./api";
import { strmLibraryPath, useStrmDetail } from "./queries";
import {
  formatStrmDate,
  formatStrmSize,
  PageBack,
  StatusChip,
  StrmEmpty,
  StrmError,
  StrmLoading,
  StrmPanel,
  strmReturnTo,
} from "./strm-ui";

const fileStates = {
  present: { label: "文件存在", tone: "success", notice: "文件内容为空。" },
  missing: {
    label: "文件缺失",
    tone: "danger",
    notice: "本地文件已缺失，无法预览。",
  },
  modified: {
    label: "文件已变更",
    tone: "warning",
    notice: "本地内容与生成时的记录不一致。",
  },
  unavailable: {
    label: "文件不可读取",
    tone: "danger",
    notice: "无法读取本地文件。",
  },
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

function FileMetadata({ file }: { file: StrmDetail }) {
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
    { label: "本地路径", value: file.output_path },
    { label: "源文件", value: file.source_path },
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
          <StatusChip tone="accent">
            {file.library_name ?? "媒体库已删除"}
          </StatusChip>
        </FileMetadataField>
        <FileMetadataField label="生成时间">
          <time dateTime={file.generated_at}>
            {formatStrmDate(file.generated_at, true)}
          </time>
        </FileMetadataField>
        <FileMetadataField label="文件大小">
          {formatStrmSize(file.size_bytes)}
        </FileMetadataField>
        <FileMetadataField label="当前状态">
          <StatusChip tone={fileStates[file.file_state].tone}>
            {fileStates[file.file_state].label}
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

function SourceLocation({ file }: { file: StrmDetail }) {
  return (
    <StrmPanel title="源文件记录" icon={Folder}>
      <dl className="space-y-2 text-sm">
        <FileMetadataField label="源文件 ID">
          <span className="wrap-anywhere">{file.source_file_id}</span>
        </FileMetadataField>
        <FileMetadataField label="源目录">
          <span className="wrap-anywhere">
            {file.source_path.slice(0, file.source_path.lastIndexOf("/") + 1) ||
              "/"}
          </span>
        </FileMetadataField>
      </dl>
    </StrmPanel>
  );
}

function FileStatuses() {
  const statuses = [
    { label: "源文件关联", icon: CircleCheck, checked: true },
    { label: "播放链接", icon: PlayCircle, checked: false },
    { label: "有效性检查", icon: ShieldCheck, checked: false },
  ];
  return (
    <StrmPanel title="关联与状态" icon={SquareCode}>
      <dl className="divide-y divide-border text-sm">
        {statuses.map(({ label, icon: Icon, checked }) => (
          <div
            key={label}
            className="flex flex-wrap items-center justify-between gap-2 py-2.5 first:pt-0 last:pb-0"
          >
            <dt className="inline-flex items-center gap-2.5 font-medium">
              <Icon aria-hidden="true" className="size-5 text-muted" />
              {label}
            </dt>
            <dd>
              <StatusChip tone={checked ? "success" : "neutral"}>
                {checked ? "已登记" : "未检查"}
              </StatusChip>
            </dd>
          </div>
        ))}
      </dl>
    </StrmPanel>
  );
}

function FilePreview({ file }: { file: StrmDetail }) {
  return (
    <StrmPanel title="STRM 内容预览" icon={SquareCode}>
      <Surface variant="secondary">
        <div className="flex min-h-28 min-w-0 items-start justify-between gap-2 p-3 sm:p-4">
          {file.content ? (
            <pre className="min-w-0 flex-1 font-mono text-sm leading-relaxed wrap-anywhere whitespace-pre-wrap">
              <code>{file.content}</code>
            </pre>
          ) : (
            <p className="text-sm text-muted">
              {fileStates[file.file_state].notice}
            </p>
          )}
          <Chip className="shrink-0" size="sm" variant="secondary">
            只读
          </Chip>
        </div>
      </Surface>
    </StrmPanel>
  );
}

export function StrmDetailPage() {
  const { id } = useParams();
  const navigationState: unknown = useLocation().state;
  const query = useStrmDetail(id);
  const file = query.data;
  const backHref = strmReturnTo(
    navigationState,
    file ? strmLibraryPath(file.library_id) : "/strm",
  );
  const notFound =
    query.error instanceof ApiError && query.error.status === 404;

  return (
    <div className="space-y-3">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <PageBack href={backHref} label="返回 STRM 管理" />
        <h1 className="m-0 text-2xl font-bold tracking-tight sm:text-[1.75rem]">
          文件详情
        </h1>
      </header>
      {notFound && (
        <StrmEmpty
          title="找不到此文件"
          description="没有找到对应的 STRM 登记记录。"
        />
      )}
      {query.isError && !notFound && (
        <StrmError error={query.error} onRetry={() => void query.refetch()} />
      )}
      {query.isPending && <StrmLoading>正在加载文件详情…</StrmLoading>}
      {file && !notFound && (
        <div className="grid items-start gap-3 lg:grid-cols-2">
          <div className="grid min-w-0 gap-3">
            <FileMetadata key={file.id} file={file} />
            <SourceLocation file={file} />
          </div>
          <div className="grid min-w-0 gap-3">
            <FilePreview file={file} />
            <FileStatuses />
          </div>
        </div>
      )}
    </div>
  );
}
