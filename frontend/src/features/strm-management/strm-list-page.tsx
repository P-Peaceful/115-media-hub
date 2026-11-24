import { Button, Card, Checkbox, Pagination } from "@heroui/react";
import {
  CircleCheck,
  Clock,
  Download,
  File,
  RefreshCw,
  ShieldCheck,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { Fragment, useState } from "react";

import { PageBanner } from "@/components/page-banner";
import { useLibraryList } from "@/features/media-library/use-library-list";

import type { StrmFile } from "./api";
import { useStrmFiles } from "./queries";
import { StrmFileTable } from "./strm-file-table";
import { maxPage, pageSizes, useStrmListQuery } from "./strm-list-query";
import {
  formatStrmDate,
  MetricTile,
  StrmEmpty,
  StrmError,
  StrmLoading,
  StrmSearch,
  StrmSelect,
} from "./strm-ui";

const countFormat = new Intl.NumberFormat("zh-CN");
const unavailableActions = [
  { label: "批量检查", icon: ShieldCheck },
  { label: "批量重新生成", icon: RefreshCw },
  { label: "批量清理", icon: Trash2 },
];

type ListQuery = ReturnType<typeof useStrmListQuery>;

function FileFilters({ query }: { query: ListQuery }) {
  const libraries = useLibraryList();
  const options: Record<string, string> = { all: "全部媒体库" };
  for (const library of libraries.data ?? []) {
    options[library.id] = library.name;
  }
  if (query.libraryId && !Object.hasOwn(options, query.libraryId)) {
    options[query.libraryId] = "指定媒体库";
  }

  return (
    <Card>
      <Card.Content>
        <div className="@container">
          <div className="grid grid-cols-2 items-center gap-2.5 @min-[47rem]:grid-cols-[minmax(14rem,2fr)_minmax(10rem,1.15fr)_minmax(8rem,1fr)_minmax(8rem,1fr)_auto]">
            <div className="col-span-2 @min-[47rem]:col-span-1">
              <StrmSearch
                value={query.search}
                onChange={query.setSearch}
                placeholder="按名称搜索（文件名/影片名）"
              />
            </div>
            <StrmSelect
              label="媒体库"
              value={query.libraryId || "all"}
              options={options}
              onChange={(id) => query.setLibrary(id === "all" ? "" : id)}
            />
            <StrmSelect
              label="状态"
              value="all"
              options={{ all: "全部状态" }}
              disabledReason="有效性检查暂未开放"
            />
            <StrmSelect
              label="时间"
              value="all"
              options={{ all: "全部时间" }}
              disabledReason="生成时间筛选暂未开放"
            />
            <div className="justify-self-end">
              <Button variant="outline" size="sm" onPress={query.reset}>
                重置
              </Button>
            </div>
          </div>
        </div>
        {libraries.isError && (
          <div className="mt-2">
            <Button
              variant="ghost"
              size="sm"
              onPress={() => void libraries.refetch()}
            >
              重新加载媒体库
            </Button>
          </div>
        )}
      </Card.Content>
    </Card>
  );
}

function csvCell(value: string) {
  const text = /^(?:\s*[=+\-@＝＋－＠]|[\t\r\n])/u.test(value)
    ? "'" + value
    : value;
  return '"' + text.replaceAll('"', '""') + '"';
}

function exportFiles(files: readonly StrmFile[]) {
  const rows = [
    ["文件名称", "所属媒体库", "生成时间"],
    ...files.map((file) => [
      file.relative_path,
      file.library_name ?? "媒体库已删除",
      formatStrmDate(file.generated_at),
    ]),
  ];
  const csv = rows.map((row) => row.map(csvCell).join(",")).join("\r\n");
  const url = URL.createObjectURL(
    new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" }),
  );
  const download = document.createElement("a");
  download.href = url;
  download.download = "strm-files.csv";
  document.body.append(download);
  download.click();
  download.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

function FileActions({
  files,
  selected,
  onSelectionChange,
}: {
  files: readonly StrmFile[];
  selected: readonly StrmFile[];
  onSelectionChange: (ids: Set<string>) => void;
}) {
  const selectionCount = selected.length;
  const exportScope = selectionCount ? "导出已选文件" : "导出当前页文件";

  return (
    <div className="grid grid-cols-[1fr_auto] items-center gap-2.5 @min-[42rem]:grid-cols-[auto_1fr_auto] @min-[42rem]:gap-x-6">
      <div>
        <Checkbox
          aria-label="选择本页全部文件"
          variant="secondary"
          isDisabled={!files.length}
          isSelected={files.length > 0 && selectionCount === files.length}
          isIndeterminate={selectionCount > 0 && selectionCount < files.length}
          onChange={(checked) =>
            onSelectionChange(
              new Set(checked ? files.map((file) => file.id) : []),
            )
          }
        >
          <Checkbox.Content>
            <Checkbox.Control>
              <Checkbox.Indicator />
            </Checkbox.Control>
            <span
              role="status"
              className="text-sm whitespace-nowrap text-muted"
            >
              已选择 {countFormat.format(selectionCount)} 项
            </span>
          </Checkbox.Content>
        </Checkbox>
      </div>
      <div className="col-span-2 row-start-2 flex flex-wrap items-center gap-2 @min-[42rem]:col-span-1 @min-[42rem]:col-start-2 @min-[42rem]:row-start-1">
        {unavailableActions.map(({ label, icon: Icon }) => (
          <span key={label} title={label + "暂未开放"}>
            <Button
              variant="outline"
              size="sm"
              isDisabled
              aria-description={label + "暂未开放"}
            >
              <Icon aria-hidden="true" />
              {label}
            </Button>
          </span>
        ))}
      </div>
      <div className="justify-self-end" title={exportScope}>
        <Button
          variant="outline"
          size="sm"
          isDisabled={!files.length}
          aria-description={exportScope}
          onPress={() => exportFiles(selectionCount ? selected : files)}
        >
          <Download aria-hidden="true" />
          导出列表
        </Button>
      </div>
    </div>
  );
}

function FilePagination({ query, total }: { query: ListQuery; total: number }) {
  const { page, limit, setPage } = query;
  const lastPage = Math.min(maxPage, Math.max(1, Math.ceil(total / limit)));
  const firstVisible = Math.max(1, Math.min(page - 2, lastPage - 4));
  const visiblePages = Array.from(
    { length: Math.min(lastPage, 5) },
    (_, index) => firstVisible + index,
  );
  const pageNumbers = Array.from(new Set([1, ...visiblePages, lastPage]));
  const activePage = page <= lastPage ? page : undefined;

  return (
    <div className="@container">
      <div className="grid grid-cols-[1fr_auto] items-center gap-2.5 @min-[43rem]:grid-cols-[1fr_auto_1fr]">
        <p className="text-sm whitespace-nowrap text-muted">
          共 {countFormat.format(total)} 条
        </p>
        <div className="col-span-2 row-start-2 max-w-full justify-self-center overflow-x-auto @min-[43rem]:col-span-1 @min-[43rem]:col-start-2 @min-[43rem]:row-start-1">
          <Pagination aria-label="STRM 文件分页" size="sm">
            <Pagination.Content>
              <Pagination.Item>
                <Pagination.Previous
                  aria-label="上一页"
                  isDisabled={page <= 1}
                  onPress={() => setPage(Math.min(page - 1, lastPage))}
                >
                  <Pagination.PreviousIcon />
                </Pagination.Previous>
              </Pagination.Item>
              {pageNumbers.map((index, position) => (
                <Fragment key={index}>
                  {index - (pageNumbers[position - 1] ?? index) > 1 && (
                    <Pagination.Item>
                      <Pagination.Ellipsis />
                    </Pagination.Item>
                  )}
                  <Pagination.Item>
                    <Pagination.Link
                      aria-label={"第 " + index + " 页"}
                      isActive={index === activePage}
                      onPress={() => setPage(index)}
                    >
                      {index}
                    </Pagination.Link>
                  </Pagination.Item>
                </Fragment>
              ))}
              <Pagination.Item>
                <Pagination.Next
                  aria-label="下一页"
                  isDisabled={page >= lastPage}
                  onPress={() => setPage(page + 1)}
                >
                  <Pagination.NextIcon />
                </Pagination.Next>
              </Pagination.Item>
            </Pagination.Content>
          </Pagination>
        </div>
        <div className="w-32 justify-self-end">
          <StrmSelect
            label="每页"
            options={pageSizes}
            value={String(limit)}
            onChange={query.setSize}
            inline
          />
        </div>
      </div>
    </div>
  );
}

function FileResults({
  files,
  query,
}: {
  files: ReturnType<typeof useStrmFiles>;
  query: ListQuery;
}) {
  const [selection, setSelection] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  const items = files.data?.items ?? [];
  const selectedFiles = items.filter((file) => selection.has(file.id));
  const selectedIds = new Set(selectedFiles.map((file) => file.id));
  const filtered = !!(query.libraryId || query.search.trim());

  return (
    <Card>
      <Card.Content>
        <div className="@container grid min-w-0 gap-3">
          <FileActions
            files={items}
            selected={selectedFiles}
            onSelectionChange={setSelection}
          />
          {files.isError && (
            <StrmError
              error={files.error}
              onRetry={() => void files.refetch()}
            />
          )}
          {files.isPending && <StrmLoading>正在加载 STRM 文件…</StrmLoading>}
          {files.data && (
            <>
              {items.length > 0 ? (
                <StrmFileTable
                  files={items}
                  query={query}
                  selectedIds={selectedIds}
                  onSelectionChange={setSelection}
                />
              ) : (
                <StrmEmpty
                  title={
                    files.data.total > 0
                      ? "这一页没有 STRM 文件"
                      : filtered
                        ? "没有符合条件的 STRM 文件"
                        : "暂无 STRM 文件"
                  }
                  description={
                    files.data.total > 0
                      ? "请选择其他页。"
                      : filtered
                        ? "可以调整搜索条件或切换媒体库。"
                        : "从媒体库详情运行全量同步后，文件将显示在这里。"
                  }
                  {...(!filtered && !files.data.total
                    ? { href: "/libraries", label: "前往媒体库" }
                    : {})}
                />
              )}
              <FilePagination query={query} total={files.data.total} />
            </>
          )}
        </div>
      </Card.Content>
    </Card>
  );
}

export function StrmListPage() {
  const query = useStrmListQuery();
  const files = useStrmFiles(query.filters);
  const total = files.data ? countFormat.format(files.data.total) : "—";

  return (
    <div className="@container grid min-w-0 gap-3">
      <PageBanner
        compact
        title="STRM 管理"
        description="管理媒体库生成的 STRM 文件，查看生成记录与源文件关联。"
      />
      <div className="grid grid-cols-2 gap-3 @min-[39rem]:grid-cols-4">
        <MetricTile label="STRM 文件" value={total} icon={File} tone="accent" />
        <MetricTile label="有效" value="—" icon={CircleCheck} tone="success" />
        <MetricTile label="异常" value="—" icon={TriangleAlert} tone="danger" />
        <MetricTile label="待处理" value={total} icon={Clock} tone="warning" />
      </div>
      <FileFilters query={query} />
      <FileResults
        key={JSON.stringify(query.filters)}
        files={files}
        query={query}
      />
    </div>
  );
}
