import { Button, Card, Pagination } from "@heroui/react";
import {
  CircleCheck,
  Clock3,
  Download,
  File,
  RefreshCw,
  ShieldCheck,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { Fragment } from "react";
import { useLocation, useNavigate } from "react-router";

import { PageBanner } from "@/components/page-banner";

import type { StrmFile } from "./strm-demo";
import {
  canRegenerateStrmFile,
  downloadStrmCsv,
  formatStrmDate,
  strmLibraries,
  strmPlaybackStatusLabels,
  strmSourceStatusLabels,
  strmStatusLabels,
  summarizeStrmFiles,
} from "./strm-demo";
import { FileCheckbox, StrmFileTable } from "./strm-file-table";
import { pageSizes, timeOptions, useStrmListQuery } from "./strm-list-query";
import type { StrmView } from "./strm-session-context";
import { useStrmSession } from "./strm-session-context";
import {
  DemoBadge,
  MetricTile,
  PageBack,
  StrmEmpty,
  StrmFeedback,
  StrmSearch,
  StrmSelect,
} from "./strm-ui";

const countFormat = new Intl.NumberFormat("zh-CN");
const libraryOptions = { all: "全部媒体库", ...strmLibraries };

function StrmStats({
  files,
  isList,
}: {
  files: readonly StrmFile[];
  isList: boolean;
}) {
  const counts = summarizeStrmFiles(files);
  return (
    <div
      className="grid grid-cols-2 gap-3 xl:grid-cols-4"
      aria-label="STRM 状态统计"
    >
      <MetricTile
        label={isList ? "STRM 文件" : "全部文件"}
        value={countFormat.format(counts.total)}
        icon={File}
        tone="accent"
      />
      <MetricTile
        label="有效"
        value={countFormat.format(counts.valid)}
        icon={CircleCheck}
        tone="success"
      />
      <MetricTile
        label="异常"
        value={countFormat.format(counts.invalid)}
        icon={TriangleAlert}
        tone="danger"
      />
      <MetricTile
        label="待处理"
        value={countFormat.format(counts.pending)}
        icon={Clock3}
        tone="warning"
      />
    </div>
  );
}

function FileFilters({
  view,
  query,
}: {
  view: StrmView;
  query: ReturnType<typeof useStrmListQuery>;
}) {
  const isList = view === "list";
  const all = isList ? "全部状态" : "全部";
  const search = (
    <StrmSearch
      value={query.query}
      onChange={(value) => query.setFilter("q", value)}
      placeholder={isList ? "按名称搜索（文件名／影片名）" : "按文件名搜索"}
    />
  );
  return (
    <Card className="strm-card">
      <Card.Content className="strm-card-row strm-filters flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2">
        {isList && <div className="strm-filter-search">{search}</div>}
        {isList && (
          <StrmSelect
            label="媒体库"
            value={query.library}
            options={libraryOptions}
            onChange={(value) => query.setFilter("library", value)}
          />
        )}
        <StrmSelect
          label="状态"
          value={query.status}
          options={{ all, ...strmStatusLabels }}
          onChange={(value) => query.setFilter("status", value)}
        />
        {isList ? (
          <StrmSelect
            label="时间"
            value={query.time}
            options={timeOptions}
            onChange={(value) => query.setFilter("time", value)}
          />
        ) : (
          <>
            <StrmSelect
              label="源文件关联"
              value={query.source}
              options={{ all, ...strmSourceStatusLabels }}
              onChange={(value) => query.setFilter("source", value)}
            />
            <StrmSelect
              label="播放链接"
              value={query.playback}
              options={{ all, ...strmPlaybackStatusLabels }}
              onChange={(value) => query.setFilter("playback", value)}
            />
            <div className="strm-filter-search">{search}</div>
          </>
        )}
        {isList && (
          <Button variant="outline" size="sm" onPress={query.reset}>
            重置
          </Button>
        )}
      </Card.Content>
    </Card>
  );
}

function FilePagination({
  query,
  findings,
}: {
  query: ReturnType<typeof useStrmListQuery>;
  findings: ReturnType<typeof summarizeStrmFiles> | null;
}) {
  const pageNumbers = Array.from(
    new Set([1, query.page - 1, query.page, query.page + 1, query.pageCount]),
  )
    .filter((page) => page >= 1 && page <= query.pageCount)
    .sort((first, second) => first - second);
  return (
    <Pagination
      aria-label="STRM 文件分页"
      size="sm"
      className="strm-pagination"
    >
      <Pagination.Summary className="text-sm text-muted">
        共 {countFormat.format(query.matching.length)} 条
        {findings && `（异常 ${findings.invalid}，待处理 ${findings.pending}）`}
      </Pagination.Summary>
      <Pagination.Content>
        <Pagination.Item>
          <Pagination.Previous
            aria-label="上一页"
            isDisabled={query.page === 1}
            onPress={() => query.setPage(query.page - 1)}
          >
            <Pagination.PreviousIcon />
          </Pagination.Previous>
        </Pagination.Item>
        {pageNumbers.map((page, index) => (
          <Fragment key={page}>
            {page - (pageNumbers[index - 1] ?? page) > 1 && (
              <Pagination.Item>
                <Pagination.Ellipsis />
              </Pagination.Item>
            )}
            <Pagination.Item>
              <Pagination.Link
                aria-label={`第 ${page} 页`}
                isActive={page === query.page}
                onPress={() => query.setPage(page)}
              >
                {page}
              </Pagination.Link>
            </Pagination.Item>
          </Fragment>
        ))}
        <Pagination.Item>
          <Pagination.Next
            aria-label="下一页"
            isDisabled={query.page >= query.pageCount}
            onPress={() => query.setPage(query.page + 1)}
          >
            <Pagination.NextIcon />
          </Pagination.Next>
        </Pagination.Item>
      </Pagination.Content>
      <StrmSelect
        label="每页"
        options={pageSizes}
        value={String(query.pageSize)}
        onChange={query.setSize}
      />
    </Pagination>
  );
}

function StrmListing({
  files,
  view,
}: {
  files: readonly StrmFile[];
  view: StrmView;
}) {
  const session = useStrmSession();
  const location = useLocation();
  const navigate = useNavigate();
  const query = useStrmListQuery(files, view);
  const isList = view === "list";
  const returnTo = location.pathname + location.search;
  const currentIds = new Set(session.files.map((file) => file.id));
  const regenerableIds = new Set(
    session.files.filter(canRegenerateStrmFile).map((file) => file.id),
  );
  const matchedIds = new Set(query.matching.map((file) => file.id));
  const selected = new Set(
    [...query.selectedIds].filter(
      (id) => matchedIds.has(id) && currentIds.has(id),
    ),
  );
  const visibleIds = query.visible
    .filter(({ id }) => currentIds.has(id))
    .map(({ id }) => id);
  const selectedOnPage = visibleIds.filter((id) => selected.has(id)).length;
  const canRegenerateSelection = [...selected].some((id) =>
    regenerableIds.has(id),
  );

  function select(ids: readonly string[], selectedOnPage: ReadonlySet<string>) {
    const next = new Set(selected);
    for (const id of ids) {
      if (selectedOnPage.has(id)) next.add(id);
      else next.delete(id);
    }
    query.setSelectedIds(next);
  }

  function check(ids: Iterable<string>) {
    session.checkFiles(ids, returnTo);
    void navigate("/strm/check-results");
  }

  function cleanup(ids: Iterable<string>) {
    session.openCleanup(ids, returnTo);
    void navigate("/strm/cleanup-preview");
  }

  return (
    <>
      <FileFilters view={view} query={query} />
      <StrmFeedback message={session.feedback} />
      <Card className="strm-card">
        {isList && (
          <div className="flex flex-wrap items-center gap-2 px-3 py-2">
            <div className="me-2 flex shrink-0 items-center gap-2 lg:me-7">
              <FileCheckbox
                label="选择当前页文件"
                isSelected={
                  selectedOnPage > 0 && selectedOnPage === visibleIds.length
                }
                isIndeterminate={
                  selectedOnPage > 0 && selectedOnPage < visibleIds.length
                }
                isDisabled={!visibleIds.length}
                onChange={(checked) =>
                  select(visibleIds, new Set(checked ? visibleIds : []))
                }
              />
              <span className="text-sm text-muted" aria-live="polite">
                已选择 {selected.size} 项
              </span>
            </div>
            <Button
              size="sm"
              variant="secondary"
              isDisabled={!selected.size}
              onPress={() => check(selected)}
            >
              <ShieldCheck aria-hidden="true" className="size-4" />
              批量检查
            </Button>
            <Button
              size="sm"
              variant="secondary"
              isDisabled={!canRegenerateSelection}
              onPress={() => session.regenerateFiles(selected)}
            >
              <RefreshCw aria-hidden="true" className="size-4" />
              批量重新生成
            </Button>
            <Button
              size="sm"
              variant="outline"
              isDisabled={!selected.size}
              onPress={() => cleanup(selected)}
            >
              <Trash2 aria-hidden="true" className="size-4" />
              批量清理
            </Button>
            <Button
              className="ms-auto"
              size="sm"
              variant="outline"
              isDisabled={!query.matching.length}
              onPress={() => downloadStrmCsv(query.matching)}
            >
              <Download aria-hidden="true" className="size-4" />
              导出列表
            </Button>
          </div>
        )}
        <div className={isList ? "px-2" : "px-2 pt-2"}>
          <StrmFileTable
            view={view}
            query={query}
            returnTo={returnTo}
            selected={selected}
            onPageSelectionChange={select}
            currentIds={currentIds}
            regenerableIds={regenerableIds}
            actions={{
              onDetails: (id) => {
                void navigate(`/strm/files/${id}`, { state: { returnTo } });
              },
              onCheck: (id) => check([id]),
              onRegenerate: (id) => {
                session.regenerateFiles([id]);
              },
              onCleanup: (id) => cleanup([id]),
            }}
          />
        </div>
        <FilePagination
          query={query}
          findings={isList ? null : summarizeStrmFiles(query.matching)}
        />
      </Card>
    </>
  );
}

export function StrmListPage() {
  const { files } = useStrmSession();
  return (
    <div className="space-y-3">
      <div className="strm-banner">
        <PageBanner
          title="STRM 管理"
          description={
            <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
              管理媒体库生成的 STRM 文件，支持有效性检查、重新生成与清理。
              <DemoBadge />
            </span>
          }
        />
      </div>
      <StrmStats files={files} isList />
      <StrmListing files={files} view="list" />
    </div>
  );
}

function formatCheckDuration(ms: number) {
  if (ms < 1000) return `${ms} 毫秒`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

export function StrmCheckResultsPage() {
  const { checkRun } = useStrmSession();
  if (!checkRun) {
    return (
      <Card className="strm-card">
        <Card.Header className="flex items-center gap-3">
          <h1>有效性检查结果</h1>
          <DemoBadge />
        </Card.Header>
        <StrmEmpty
          title="暂无检查结果"
          description="请从 STRM 管理中选择文件并运行检查。"
          href="/strm"
          label="返回 STRM 管理"
        />
      </Card>
    );
  }
  const { total, invalid, pending } = summarizeStrmFiles(checkRun.files);
  return (
    <div className="space-y-3">
      <Card className="strm-card strm-check-banner">
        <Card.Content className="strm-card-row flex-wrap items-center gap-3 px-3 py-3 sm:px-5">
          <PageBack href={checkRun.returnTo} label="返回 STRM 管理" />
          <span className="grid size-11 shrink-0 place-items-center rounded-full bg-success-soft text-success-soft-foreground">
            <ShieldCheck aria-hidden="true" className="size-6" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="strm-results-title">有效性检查完成</h1>
              <DemoBadge />
            </div>
            <p className="text-sm text-muted">
              共检查 {countFormat.format(total)} 个 STRM 文件，发现{" "}
              <strong className="text-danger-soft-foreground">
                {countFormat.format(invalid)}
              </strong>{" "}
              个异常，
              <strong className="text-warning-soft-foreground">
                {countFormat.format(pending)}
              </strong>{" "}
              个待处理。
            </p>
          </div>
          <dl className="grid shrink-0 gap-1 text-sm text-muted tabular-nums sm:text-right">
            <div className="flex gap-2 sm:justify-end">
              <dt>检查时间</dt>
              <dd>{formatStrmDate(checkRun.checkedAt, true)}</dd>
            </div>
            <div className="flex gap-2 sm:justify-end">
              <dt>耗时</dt>
              <dd>{formatCheckDuration(checkRun.durationMs)}</dd>
            </div>
          </dl>
        </Card.Content>
      </Card>
      <StrmStats files={checkRun.files} isList={false} />
      <StrmListing files={checkRun.files} view="results" />
    </div>
  );
}
