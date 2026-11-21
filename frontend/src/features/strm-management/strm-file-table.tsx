import { Button, Checkbox, Dropdown, Label, Table } from "@heroui/react";
import { FilePlay, MoreHorizontal } from "lucide-react";
import { Link } from "react-router";

import type { StrmFile } from "./strm-demo";
import {
  formatStrmDate,
  strmLibraries,
  strmPlaybackStatusLabels,
  strmSourceStatusLabels,
  strmStatusLabels,
} from "./strm-demo";
import type { useStrmListQuery } from "./strm-list-query";
import type { StrmView } from "./strm-session-context";
import { StatusChip, StrmEmpty } from "./strm-ui";

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

export function FileCheckbox({
  label,
  selectionSlot = null,
  ...props
}: {
  label: string;
  selectionSlot?: "selection" | null;
  isSelected?: boolean;
  isIndeterminate?: boolean;
  isDisabled?: boolean;
  onChange?: (selected: boolean) => void;
}) {
  return (
    <Checkbox slot={selectionSlot} aria-label={label} {...props}>
      <Checkbox.Content>
        <Checkbox.Control>
          <Checkbox.Indicator />
        </Checkbox.Control>
      </Checkbox.Content>
    </Checkbox>
  );
}

type FileActions = {
  onDetails: (id: string) => void;
  onCheck: (id: string) => void;
  onRegenerate: (id: string) => void;
  onCleanup: (id: string) => void;
};

function RowActions({
  file,
  view,
  active,
  canRegenerate,
  actions,
}: {
  file: StrmFile;
  view: StrmView;
  active: boolean;
  canRegenerate: boolean;
  actions: FileActions;
}) {
  const regenerateInline =
    view === "results" && file.status !== "valid" && canRegenerate;
  const disabledKeys = [
    ...(!active ? ["check", "cleanup"] : []),
    ...(!canRegenerate ? ["regenerate"] : []),
  ];
  return (
    <div className="flex items-center gap-1">
      {regenerateInline && (
        <Button
          size="sm"
          variant="secondary"
          className="whitespace-nowrap"
          onPress={() => actions.onRegenerate(file.id)}
        >
          重新生成
        </Button>
      )}
      <Dropdown>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label={`${file.relativePath} 的操作`}
        >
          <MoreHorizontal className="size-4" aria-hidden="true" />
        </Button>
        <Dropdown.Popover>
          <Dropdown.Menu
            disabledKeys={disabledKeys}
            onAction={(key) => {
              if (key === "details") actions.onDetails(file.id);
              if (key === "check") actions.onCheck(file.id);
              if (key === "regenerate") actions.onRegenerate(file.id);
              if (key === "cleanup") actions.onCleanup(file.id);
            }}
          >
            <Dropdown.Item id="details" textValue="查看详情">
              <Label>查看详情</Label>
            </Dropdown.Item>
            {view === "list" && (
              <Dropdown.Item id="check" textValue="检查有效性">
                <Label>检查有效性</Label>
              </Dropdown.Item>
            )}
            {!regenerateInline && (
              <Dropdown.Item id="regenerate" textValue="重新生成">
                <Label>重新生成</Label>
              </Dropdown.Item>
            )}
            <Dropdown.Item id="cleanup" textValue="清理预览">
              <Label>清理预览</Label>
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>
    </div>
  );
}

export function StrmFileTable({
  view,
  query,
  returnTo,
  selected,
  onPageSelectionChange,
  currentIds,
  regenerableIds,
  actions,
}: {
  view: StrmView;
  query: ReturnType<typeof useStrmListQuery>;
  returnTo: string;
  selected: ReadonlySet<string>;
  onPageSelectionChange: (
    ids: readonly string[],
    selected: ReadonlySet<string>,
  ) => void;
  currentIds: ReadonlySet<string>;
  regenerableIds: ReadonlySet<string>;
  actions: FileActions;
}) {
  const isList = view === "list";
  const pageIds = query.visible
    .filter((file) => currentIds.has(file.id))
    .map((file) => file.id);
  return (
    <Table className="min-w-0">
      <Table.ScrollContainer className="strm-table-scroll">
        <Table.Content
          aria-label={isList ? "STRM 文件列表" : "有效性检查结果"}
          className="strm-files-grid"
          selectionMode="multiple"
          selectedKeys={pageIds.filter((id) => selected.has(id))}
          disabledKeys={query.visible
            .filter((file) => !currentIds.has(file.id))
            .map((file) => file.id)}
          disabledBehavior="selection"
          onSelectionChange={(keys) => {
            const next =
              keys === "all" ? pageIds : pageIds.filter((id) => keys.has(id));
            onPageSelectionChange(pageIds, new Set(next));
          }}
          sortDescriptor={{
            column: query.sortColumn,
            direction: query.sortDirection,
          }}
          onSortChange={({ column, direction }) => {
            if (column === "name" || column === "generated")
              query.setSort(column, direction);
          }}
        >
          <Table.Header>
            <Table.Column id="selection" aria-label="选择" className="w-9">
              <FileCheckbox label="全选当前页" selectionSlot="selection" />
            </Table.Column>
            <Table.Column id="name" isRowHeader allowsSorting>
              {({ sortDirection }) => (
                <Table.SortableColumnHeader
                  {...(sortDirection ? { sortDirection } : {})}
                >
                  文件名称
                </Table.SortableColumnHeader>
              )}
            </Table.Column>
            {isList && <Table.Column id="library">所属媒体库</Table.Column>}
            {isList && <Table.Column id="status">状态</Table.Column>}
            {isList && (
              <Table.Column id="generated" allowsSorting>
                {({ sortDirection }) => (
                  <Table.SortableColumnHeader
                    {...(sortDirection ? { sortDirection } : {})}
                  >
                    生成时间
                  </Table.SortableColumnHeader>
                )}
              </Table.Column>
            )}
            <Table.Column id="source">源文件关联</Table.Column>
            <Table.Column id="playback">播放链接</Table.Column>
            {!isList && (
              <Table.Column id="findings">检查结果 / 异常说明</Table.Column>
            )}
            <Table.Column id="actions">操作</Table.Column>
          </Table.Header>
          <Table.Body
            items={query.visible}
            dependencies={[currentIds, regenerableIds, returnTo, actions]}
            renderEmptyState={() => (
              <StrmEmpty
                title="暂无匹配文件"
                description="可调整筛选条件查看其他 STRM 文件。"
              />
            )}
          >
            {(file) => (
              <Table.Row id={file.id}>
                <Table.Cell>
                  <FileCheckbox
                    label={`选择 ${file.relativePath}`}
                    selectionSlot="selection"
                  />
                </Table.Cell>
                <Table.Cell>
                  <Link
                    to={`/strm/files/${file.id}`}
                    state={{ returnTo }}
                    className="flex w-fit items-center gap-2 font-medium text-foreground strm-file-link"
                  >
                    <FilePlay
                      className="size-4 shrink-0 text-accent-soft-foreground"
                      aria-hidden="true"
                    />
                    {file.relativePath}
                  </Link>
                </Table.Cell>
                {isList && (
                  <Table.Cell>
                    <StatusChip tone="accent">
                      {strmLibraries[file.library]}
                    </StatusChip>
                  </Table.Cell>
                )}
                {isList && (
                  <Table.Cell>
                    <StatusChip dot tone={statusTones[file.status]}>
                      {strmStatusLabels[file.status]}
                    </StatusChip>
                  </Table.Cell>
                )}
                {isList && (
                  <Table.Cell className="whitespace-nowrap tabular-nums">
                    {formatStrmDate(file.generatedAt)}
                  </Table.Cell>
                )}
                <Table.Cell>
                  <StatusChip
                    dot
                    tone={file.sourceStatus === "linked" ? "success" : "danger"}
                  >
                    {strmSourceStatusLabels[file.sourceStatus]}
                  </StatusChip>
                </Table.Cell>
                <Table.Cell>
                  <StatusChip dot tone={playbackTones[file.playbackStatus]}>
                    {strmPlaybackStatusLabels[file.playbackStatus]}
                  </StatusChip>
                </Table.Cell>
                {!isList && (
                  <Table.Cell>
                    <div className="flex items-center gap-2">
                      <StatusChip tone={statusTones[file.status]}>
                        {strmStatusLabels[file.status]}
                      </StatusChip>
                      <span className="text-sm text-muted">
                        {file.reason ?? "—"}
                      </span>
                    </div>
                  </Table.Cell>
                )}
                <Table.Cell>
                  <RowActions
                    file={file}
                    view={view}
                    active={currentIds.has(file.id)}
                    canRegenerate={regenerableIds.has(file.id)}
                    actions={actions}
                  />
                </Table.Cell>
              </Table.Row>
            )}
          </Table.Body>
        </Table.Content>
      </Table.ScrollContainer>
    </Table>
  );
}
