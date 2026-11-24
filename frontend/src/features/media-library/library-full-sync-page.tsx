import { Button, Card, Table } from "@heroui/react";
import {
  ChevronRight,
  Clock3,
  History,
  Info,
  Layers,
  Play,
  RefreshCw,
} from "lucide-react";
import { Link, useNavigate } from "react-router";

import { formatDuration, StatusChip } from "@/features/task-center/task-ui";

import { useLibraryDetailContext } from "./library-detail-context";
import {
  SyncPageHeader,
  SyncScope,
  SyncSection,
} from "./library-sync-components";
import {
  librarySyncHistoryPath,
  SyncHistoryState,
  SyncTaskCounts,
  SyncTaskLink,
  SyncTaskTime,
  useLibrarySyncHistory,
} from "./library-sync-history";

export function LibraryFullSyncPage() {
  const { item } = useLibraryDetailContext();
  const navigate = useNavigate();
  const history = useLibrarySyncHistory(item.id);
  const records = history.data?.items ?? [];
  const latest = records[0];

  return (
    <div className="space-y-4">
      <SyncPageHeader
        title="全量同步"
        description="扫描媒体库绑定的 115 目录，按已保存的配置生成 STRM 文件。"
        icon={RefreshCw}
      />
      <div className="grid gap-4 xl:grid-cols-[1.1fr_1fr]">
        <SyncSection title="执行说明" icon={Info}>
          <ul className="list-inside list-disc space-y-2 text-sm marker:text-accent">
            <li>扫描绑定目录，并按配置处理子目录和排除目录。</li>
            <li>为识别到的视频生成 302 播放链接，并按配置处理已有 STRM。</li>
            <li>同步完成后，可在任务详情查看处理数量和失败原因。</li>
          </ul>
        </SyncSection>
        <SyncSection title="同步范围" icon={Layers}>
          <SyncScope library={item} />
        </SyncSection>
      </div>
      <SyncSection title="最近执行记录" icon={Clock3}>
        <SyncHistoryState history={history} />
        {latest && (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-surface-secondary p-3 text-sm">
            <div className="flex flex-wrap items-center gap-3">
              <StatusChip task={latest} />
              <SyncTaskTime task={latest} />
              <span>耗时 {formatDuration(latest.duration_ms)}</span>
              <SyncTaskCounts task={latest} />
            </div>
            <SyncTaskLink id={latest.id} label="查看最近任务" />
          </div>
        )}
      </SyncSection>
      {latest && (
        <SyncSection
          title="历史记录"
          icon={History}
          action={
            <Link
              className="inline-flex items-center gap-1 text-sm font-medium text-accent hover:underline"
              to={librarySyncHistoryPath(item.id)}
            >
              查看全部历史
              <ChevronRight aria-hidden="true" className="size-4" />
            </Link>
          }
        >
          <Table>
            <Table.ScrollContainer>
              <Table.Content
                aria-label="全量同步历史记录"
                className="min-w-[580px]"
              >
                <Table.Header>
                  <Table.Column isRowHeader>执行时间</Table.Column>
                  <Table.Column>执行结果</Table.Column>
                  <Table.Column>耗时</Table.Column>
                  <Table.Column>处理统计</Table.Column>
                  <Table.Column>操作</Table.Column>
                </Table.Header>
                <Table.Body>
                  {records.map((record) => (
                    <Table.Row key={record.id} id={record.id}>
                      <Table.Cell>
                        <SyncTaskTime task={record} />
                      </Table.Cell>
                      <Table.Cell>
                        <StatusChip task={record} />
                      </Table.Cell>
                      <Table.Cell className="whitespace-nowrap">
                        {formatDuration(record.duration_ms)}
                      </Table.Cell>
                      <Table.Cell>
                        <SyncTaskCounts task={record} />
                      </Table.Cell>
                      <Table.Cell>
                        <SyncTaskLink id={record.id} />
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Content>
            </Table.ScrollContainer>
          </Table>
        </SyncSection>
      )}
      <Card>
        <Card.Content className="flex items-end">
          <Button onPress={() => void navigate("execution-preview")}>
            <Play aria-hidden="true" className="size-4" />
            下一步：执行预览
          </Button>
        </Card.Content>
      </Card>
    </div>
  );
}
