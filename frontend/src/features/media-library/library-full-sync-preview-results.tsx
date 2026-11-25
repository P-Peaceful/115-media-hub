import { Alert, Table } from "@heroui/react";

import { formatCount } from "@/features/task-center/task-ui";

import type {
  FullSyncOutputCheck,
  FullSyncPreview,
} from "./full-sync-preview-query";

const outputLabels: Record<FullSyncOutputCheck["status"], string> = {
  ready: "输出目录可用",
  will_create: "同步时创建输出目录",
  missing: "输出目录或上级目录不存在",
  not_directory: "输出路径不可用",
  permission_denied: "输出目录权限不足",
  unavailable: "无法访问输出目录",
};

const stopMessages: Record<FullSyncPreview["stop_reason"], string> = {
  scope_complete: "已读完当前配置的源目录范围。",
  directory_limit: "已达到 3 个目录的抽样上限。",
  request_limit: "已达到 5 次目录请求的抽样上限（包含重试）。",
  time_limit: "已达到 15 秒的抽样时间上限。",
  sample_limit: "已达到 20 条文件样例的抽样上限。",
  source_error: "部分源目录无法完整读取。",
};

export function canUsePreviewOutput({ status }: FullSyncOutputCheck) {
  return status === "ready" || status === "will_create";
}

export function FullSyncPreviewResults({
  preview,
}: {
  preview: FullSyncPreview;
}) {
  const visibleCounts = [
    ["抽样发现的视频", preview.scanned_videos],
    ["抽样 STRM 候选", preview.candidate_count],
    ["抽样异常", preview.failed_count],
  ] as const;
  const { output, samples } = preview;
  const outputLabel = outputLabels[output.status];
  const outputStatus =
    output.status === "ready"
      ? "success"
      : output.status === "will_create"
        ? "accent"
        : "danger";

  return (
    <div className="space-y-4">
      {!preview.scan_complete && (
        <Alert status="warning" role="status">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>抽样结果（未覆盖全部范围）</Alert.Title>
            <Alert.Description>
              {stopMessages[preview.stop_reason]}{" "}
              当前数量仅为抽样已见结果，不代表全库总量。确认后会完整扫描媒体源。
            </Alert.Description>
          </Alert.Content>
        </Alert>
      )}
      <dl className="grid grid-cols-3 gap-2 sm:gap-3">
        {visibleCounts.map(([label, count]) => (
          <div
            key={label}
            className="flex min-w-0 flex-col justify-between gap-1 rounded-xl bg-surface-secondary p-3"
          >
            <dt className="text-xs text-muted sm:text-sm">{label}</dt>
            <dd className="font-semibold tabular-nums">{formatCount(count)}</dd>
          </div>
        ))}
      </dl>
      <Alert
        status={outputStatus}
        role={canUsePreviewOutput(output) ? "status" : "alert"}
      >
        <Alert.Indicator />
        <Alert.Content>
          <Alert.Title>{outputLabel}</Alert.Title>
          {output.message !== outputLabel && (
            <Alert.Description className="wrap-anywhere">
              {output.message}
            </Alert.Description>
          )}
        </Alert.Content>
      </Alert>
      {samples.length > 0 ? (
        <Table>
          <Table.ScrollContainer className="max-h-80">
            <Table.Content
              aria-label="候选文件示例"
              className="w-full table-fixed"
            >
              <Table.Header>
                <Table.Column isRowHeader>115 源文件示例</Table.Column>
                <Table.Column>STRM 输出相对路径</Table.Column>
              </Table.Header>
              <Table.Body>
                {samples.map(({ source_path, relative_path }, index) => {
                  const key = `${index}:${relative_path}`;
                  return (
                    <Table.Row key={key} id={key}>
                      <Table.Cell className="wrap-anywhere">
                        {source_path}
                      </Table.Cell>
                      <Table.Cell className="wrap-anywhere">
                        {relative_path}
                      </Table.Cell>
                    </Table.Row>
                  );
                })}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
      ) : (
        <p className="text-sm text-muted">
          本次抽样未发现可生成 STRM 的视频文件。
        </p>
      )}
    </div>
  );
}
