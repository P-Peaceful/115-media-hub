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

export function canUsePreviewOutput({ status }: FullSyncOutputCheck) {
  return status === "ready" || status === "will_create";
}

export function FullSyncPreviewResults({
  preview,
}: {
  preview: FullSyncPreview;
}) {
  const visibleCounts = [
    ["扫描到的视频", preview.scanned_videos],
    ["STRM 候选文件", preview.candidate_count],
    ["扫描异常", preview.failed_count],
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
            <Alert.Title>部分扫描结果</Alert.Title>
            <Alert.Description>
              预览扫描已达到时间上限，当前数量仅统计已扫描的部分。确认后会完整扫描媒体源。
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
      ) : preview.scan_complete ? (
        <p className="text-sm text-muted">未扫描到可生成 STRM 的视频文件。</p>
      ) : null}
    </div>
  );
}
