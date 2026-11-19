import { Card, Table, ToggleButton, ToggleButtonGroup } from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import {
  FastForward,
  Info,
  ListChecks,
  Play,
  Plus,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { Link, useSearchParams } from "react-router";

import {
  libraryDetailPath,
  useLibraryDetailContext,
} from "./library-detail-context";
import {
  ChangeChip,
  DemoExecutionAction,
  SyncCounts,
  SyncPageHeader,
  SyncSection,
} from "./library-sync-components";
import {
  changeKinds,
  changeLabels,
  estimatedChanges,
  estimatedTotal,
  formatSyncCount,
  previewChanges,
  sampleMediaPath,
} from "./library-sync-demo";

const metrics = {
  added: {
    icon: Plus,
    hint: "将新增媒体文件",
    style: "bg-success-soft text-success",
  },
  updated: {
    icon: RefreshCw,
    hint: "将更新元数据或文件",
    style: "bg-accent-soft text-accent",
  },
  skipped: {
    icon: FastForward,
    hint: "内容无变化，跳过处理",
    style: "bg-surface-secondary text-muted",
  },
  deleted: {
    icon: Trash2,
    hint: "将移除已不存在的媒体",
    style: "bg-danger-soft text-danger",
  },
};

const previewLimit = 20;

export function LibraryExecutionPreviewPage() {
  const { item } = useLibraryDetailContext();
  const [searchParams, setSearchParams] = useSearchParams();
  const filter =
    changeKinds.find((kind) => kind === searchParams.get("change")) ?? "all";
  const changes = previewChanges
    .filter((change) => filter === "all" || change.kind === filter)
    .slice(0, previewLimit);

  return (
    <div className="space-y-4">
      <SyncPageHeader
        title="执行预览"
        description="核对本次全量同步预计影响的媒体文件，确认后执行。"
        icon={Play}
        backToFullSync
      />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {changeKinds.map((kind) => {
          const { icon: Icon, hint, style } = metrics[kind];
          return (
            <Card key={kind} className="min-w-0 border border-border shadow-sm">
              <Card.Content>
                <dl>
                  <dt className="flex items-center gap-2 text-sm font-semibold sm:text-base">
                    <span
                      className={`flex size-7 shrink-0 items-center justify-center rounded-lg ${style}`}
                    >
                      <Icon aria-hidden="true" className="size-4" />
                    </span>
                    预计{changeLabels[kind]}
                  </dt>
                  <dd className="mt-2 text-xl font-bold tabular-nums sm:text-3xl">
                    {formatSyncCount(estimatedChanges[kind])}
                  </dd>
                  <dd className="mt-1 text-xs text-muted sm:text-sm">{hint}</dd>
                </dl>
              </Card.Content>
            </Card>
          );
        })}
      </div>
      <SyncSection
        title="影响范围预览"
        icon={ListChecks}
        action={
          <ToggleButtonGroup
            aria-label="筛选变更类型"
            selectionMode="single"
            disallowEmptySelection
            isDetached
            className="flex flex-wrap gap-1"
            size="sm"
            selectedKeys={new Set([filter])}
            onSelectionChange={(keys) => {
              const key = [...keys][0];
              setSearchParams((current) => {
                const next = new URLSearchParams(current);
                if (typeof key === "string" && key !== "all") {
                  next.set("change", key);
                } else {
                  next.delete("change");
                }
                return next;
              });
            }}
          >
            <ToggleButton id="all">
              全部 ({formatSyncCount(estimatedTotal)})
            </ToggleButton>
            {changeKinds.map((kind) => (
              <ToggleButton key={kind} id={kind}>
                {changeLabels[kind]} ({formatSyncCount(estimatedChanges[kind])})
              </ToggleButton>
            ))}
          </ToggleButtonGroup>
        }
      >
        <Table>
          <Table.ScrollContainer className="max-h-72 overflow-auto">
            <Table.Content aria-label="影响范围预览" className="min-w-[660px]">
              <Table.Header>
                <Table.Column>序号</Table.Column>
                <Table.Column>变更类型</Table.Column>
                <Table.Column isRowHeader>媒体名称</Table.Column>
                <Table.Column>路径</Table.Column>
                <Table.Column>详情</Table.Column>
              </Table.Header>
              <Table.Body>
                {changes.map((change, index) => (
                  <Table.Row key={change.title} id={change.title}>
                    <Table.Cell>{index + 1}</Table.Cell>
                    <Table.Cell>
                      <ChangeChip kind={change.kind} />
                    </Table.Cell>
                    <Table.Cell>{change.title}</Table.Cell>
                    <Table.Cell className="max-w-56 wrap-anywhere">
                      {sampleMediaPath(item.directory_path, change.entry)}
                    </Table.Cell>
                    <Table.Cell>{change.detail}</Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
        <p className="mt-2 text-xs text-muted">
          每种筛选条件最多显示前 {previewLimit} 条样例。
        </p>
      </SyncSection>
      <DemoExecutionAction
        label="确认执行"
        secondaryAction={
          <Link
            className={buttonVariants({ variant: "secondary" })}
            to={`${libraryDetailPath(item.id)}/full-sync`}
          >
            取消
          </Link>
        }
        summary={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="inline-flex items-center gap-2">
              <Info aria-hidden="true" className="size-4 text-accent" />
              预计总计{" "}
              <strong className="tabular-nums">
                {formatSyncCount(estimatedTotal)}
              </strong>{" "}
              个文件
            </span>
            <span className="text-muted">
              <SyncCounts counts={estimatedChanges} />
            </span>
          </div>
        }
      />
    </div>
  );
}
