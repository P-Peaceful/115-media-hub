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
import { useRef, useState } from "react";
import { useNavigate } from "react-router";

import { useLibraryDetailContext } from "./library-detail-context";
import {
  LibraryLocalTime,
  ResultChip,
  SyncCounts,
  SyncPageHeader,
  SyncRecordDetails,
  SyncSection,
} from "./library-sync-components";
import {
  estimatedTotal,
  formatSyncCount,
  fullSyncHistory,
} from "./library-sync-demo";

export function LibraryFullSyncPage() {
  const { item } = useLibraryDetailContext();
  const navigate = useNavigate();
  const [allRecords, setAllRecords] = useState(false);
  const [selected, setSelected] = useState<(typeof fullSyncHistory)[number]>();
  const detailsOrigin = useRef<HTMLElement | null>(null);
  const historyToggle = useRef<HTMLButtonElement>(null);
  const latest = fullSyncHistory[0];
  const records = allRecords ? fullSyncHistory : fullSyncHistory.slice(0, 3);

  return (
    <div className="space-y-4">
      <SyncPageHeader
        title="全量同步"
        description="完整扫描绑定目录，重新识别媒体文件和元数据。"
        icon={RefreshCw}
      />
      <div className="grid gap-4 xl:grid-cols-[1.1fr_1fr]">
        <SyncSection title="执行说明" icon={Info}>
          <ul className="list-inside list-disc space-y-2 text-sm marker:text-accent">
            <li>完整扫描媒体库绑定的目录。</li>
            <li>识别媒体文件，按输出配置更新 STRM、NFO 和海报。</li>
            <li>适合首次配置完成或目录结构大幅变更后使用。</li>
          </ul>
        </SyncSection>
        <SyncSection title="同步范围" icon={Layers}>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 rounded-xl bg-surface-secondary p-3 text-sm">
            <dt className="text-muted">媒体源目录</dt>
            <dd className="wrap-anywhere font-medium">{item.directory_path}</dd>
            <dt className="text-muted">输出目录</dt>
            <dd className="wrap-anywhere font-medium">{item.output_path}</dd>
            <dt className="text-muted">预计处理</dt>
            <dd className="font-medium">
              约 {formatSyncCount(estimatedTotal)} 个文件（演示）
            </dd>
          </dl>
        </SyncSection>
      </div>
      <SyncSection title="最近执行记录" icon={Clock3}>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-surface-secondary p-3 text-sm">
          <div className="flex flex-wrap items-center gap-3">
            <ResultChip result={latest.result} />
            <span>
              上次执行 <LibraryLocalTime at={latest.at} />
            </span>
            <span>全量同步</span>
            <span className="whitespace-nowrap">耗时 {latest.duration}</span>
            <SyncCounts counts={latest.counts} />
          </div>
          <Button
            size="sm"
            variant="secondary"
            onPress={({ target }) => {
              detailsOrigin.current =
                target instanceof HTMLElement ? target : null;
              setSelected(latest);
            }}
          >
            查看详情
            <ChevronRight aria-hidden="true" className="size-4" />
          </Button>
        </div>
      </SyncSection>
      <SyncSection
        title="历史记录"
        icon={History}
        action={
          <Button
            ref={historyToggle}
            size="sm"
            variant="ghost"
            aria-expanded={allRecords}
            onPress={() => setAllRecords(!allRecords)}
          >
            {allRecords ? "收起" : "查看全部"}
            <ChevronRight aria-hidden="true" className="size-4" />
          </Button>
        }
      >
        {selected && (
          <SyncRecordDetails
            key={selected.at}
            title="执行详情"
            closeLabel="返回记录"
            onClose={() => {
              setSelected(undefined);
              const origin = detailsOrigin.current;
              (origin?.isConnected ? origin : historyToggle.current)?.focus();
            }}
          >
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-2 text-sm">
              <dt className="text-muted">执行时间</dt>
              <dd>
                <LibraryLocalTime at={selected.at} />
              </dd>
              <dt className="text-muted">执行类型</dt>
              <dd>全量同步</dd>
              <dt className="text-muted">执行结果</dt>
              <dd>
                <ResultChip result={selected.result} />
              </dd>
              <dt className="text-muted">耗时</dt>
              <dd>{selected.duration}</dd>
              <dt className="text-muted">处理统计</dt>
              <dd>
                <SyncCounts counts={selected.counts} />
              </dd>
            </dl>
          </SyncRecordDetails>
        )}
        <Table>
          <Table.ScrollContainer>
            <Table.Content
              aria-label="全量同步历史记录"
              className="min-w-[745px]"
            >
              <Table.Header>
                <Table.Column isRowHeader>执行时间</Table.Column>
                <Table.Column>执行类型</Table.Column>
                <Table.Column>执行结果</Table.Column>
                <Table.Column>耗时</Table.Column>
                <Table.Column>处理统计</Table.Column>
                <Table.Column>操作</Table.Column>
              </Table.Header>
              <Table.Body>
                {records.map((record) => (
                  <Table.Row key={record.at} id={record.at}>
                    <Table.Cell>
                      <LibraryLocalTime at={record.at} />
                    </Table.Cell>
                    <Table.Cell>全量同步</Table.Cell>
                    <Table.Cell>
                      <ResultChip result={record.result} />
                    </Table.Cell>
                    <Table.Cell className="whitespace-nowrap">
                      {record.duration}
                    </Table.Cell>
                    <Table.Cell>
                      <SyncCounts counts={record.counts} />
                    </Table.Cell>
                    <Table.Cell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onPress={({ target }) => {
                          detailsOrigin.current =
                            target instanceof HTMLElement ? target : null;
                          setSelected(record);
                        }}
                      >
                        详情
                      </Button>
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
      </SyncSection>
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
