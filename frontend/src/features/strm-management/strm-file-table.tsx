import {
  Button,
  Checkbox,
  Dropdown,
  Label,
  Link,
  RouterProvider,
  Table,
} from "@heroui/react";
import { Ellipsis, FilePlay } from "lucide-react";
import { useLocation, useNavigate } from "react-router";

import type { StrmFile } from "./api";
import type { useStrmListQuery } from "./strm-list-query";
import { formatStrmDate, StatusChip } from "./strm-ui";

function FileSelection({ label }: { label: string }) {
  return (
    <Checkbox slot="selection" aria-label={label} variant="secondary">
      <Checkbox.Content>
        <Checkbox.Control>
          <Checkbox.Indicator />
        </Checkbox.Control>
      </Checkbox.Content>
    </Checkbox>
  );
}

export function StrmFileTable({
  files,
  query,
  selectedIds,
  onSelectionChange,
}: {
  files: StrmFile[];
  query: ReturnType<typeof useStrmListQuery>;
  selectedIds: ReadonlySet<string>;
  onSelectionChange: (ids: Set<string>) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const returnTo = location.pathname + location.search;
  const detailPath = (id: string) => `/strm/files/${encodeURIComponent(id)}`;
  const openDetail = (path: string) => {
    void navigate(path, { state: { returnTo } });
  };

  return (
    <RouterProvider navigate={openDetail}>
      <Table>
        <Table.ScrollContainer className="max-h-[clamp(15rem,42vh,25rem)]">
          <Table.Content
            aria-label="STRM 文件列表"
            className="min-w-4xl"
            selectionMode="multiple"
            selectedKeys={selectedIds}
            onSelectionChange={(keys) =>
              onSelectionChange(
                new Set(
                  keys === "all"
                    ? files.map((file) => file.id)
                    : [...keys].map(String),
                ),
              )
            }
            onRowAction={(id) => openDetail(detailPath(String(id)))}
            sortDescriptor={{
              column: query.sort,
              direction: query.order === "asc" ? "ascending" : "descending",
            }}
            onSortChange={({ column, direction }) => {
              if (column === "name" || column === "generated") {
                query.setSort(
                  column,
                  direction === "ascending" ? "asc" : "desc",
                );
              }
            }}
          >
            <Table.Header>
              <Table.Column id="selection">
                <FileSelection label="全选当前页文件" />
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
              <Table.Column id="library">所属媒体库</Table.Column>
              <Table.Column id="status">状态</Table.Column>
              <Table.Column id="generated" allowsSorting>
                {({ sortDirection }) => (
                  <Table.SortableColumnHeader
                    {...(sortDirection ? { sortDirection } : {})}
                  >
                    生成时间
                  </Table.SortableColumnHeader>
                )}
              </Table.Column>
              <Table.Column id="source">源文件关联</Table.Column>
              <Table.Column id="playback">播放链接</Table.Column>
              <Table.Column id="actions">操作</Table.Column>
            </Table.Header>
            <Table.Body items={files} dependencies={[returnTo]}>
              {(file) => (
                <Table.Row id={file.id}>
                  <Table.Cell>
                    <FileSelection label={`选择 ${file.relative_path}`} />
                  </Table.Cell>
                  <Table.Cell>
                    <Link href={detailPath(file.id)}>
                      <span className="flex items-center gap-2">
                        <FilePlay
                          className="size-4 shrink-0 text-accent-soft-foreground"
                          aria-hidden="true"
                        />
                        <span className="wrap-anywhere">
                          {file.relative_path}
                        </span>
                      </span>
                    </Link>
                  </Table.Cell>
                  <Table.Cell>
                    <StatusChip tone={file.library_name ? "accent" : "neutral"}>
                      {file.library_name ?? "媒体库已删除"}
                    </StatusChip>
                  </Table.Cell>
                  <Table.Cell>
                    <StatusChip tone="neutral" dot>
                      未检查
                    </StatusChip>
                  </Table.Cell>
                  <Table.Cell>
                    <time
                      className="whitespace-nowrap tabular-nums"
                      dateTime={file.generated_at}
                    >
                      {formatStrmDate(file.generated_at)}
                    </time>
                  </Table.Cell>
                  <Table.Cell>
                    <StatusChip tone="success" dot>
                      已登记
                    </StatusChip>
                  </Table.Cell>
                  <Table.Cell>
                    <StatusChip tone="neutral" dot>
                      未检查
                    </StatusChip>
                  </Table.Cell>
                  <Table.Cell>
                    <Dropdown>
                      <Button
                        aria-label={`${file.relative_path} 的操作`}
                        isIconOnly
                        size="sm"
                        variant="ghost"
                      >
                        <Ellipsis aria-hidden="true" />
                      </Button>
                      <Dropdown.Popover placement="bottom end">
                        <Dropdown.Menu aria-label="文件操作">
                          <Dropdown.Item
                            id="detail"
                            textValue="查看详情"
                            onAction={() => openDetail(detailPath(file.id))}
                          >
                            <Label>查看详情</Label>
                          </Dropdown.Item>
                        </Dropdown.Menu>
                      </Dropdown.Popover>
                    </Dropdown>
                  </Table.Cell>
                </Table.Row>
              )}
            </Table.Body>
          </Table.Content>
        </Table.ScrollContainer>
      </Table>
    </RouterProvider>
  );
}
