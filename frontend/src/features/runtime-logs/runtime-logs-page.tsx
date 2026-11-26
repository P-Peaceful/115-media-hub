import {
  Alert,
  Button,
  Card,
  Checkbox,
  Chip,
  Dropdown,
  EmptyState,
  Label,
  ListBox,
  Pagination,
  SearchField,
  Select,
  Separator,
  Spinner,
  Switch,
} from "@heroui/react";
import {
  Calendar,
  ChevronDown,
  Download,
  FileText,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { PageBanner } from "@/components/page-banner";
import { taskTypes } from "@/features/task-center/task-kinds";
import { ApiError } from "@/lib/api/errors";

import type { RuntimeLogEntry } from "./api";
import { exportRuntimeLogs } from "./api";
import { ClearLogsDialog } from "./clear-logs-dialog";
import type {
  ClearRuntimeLogsScope,
  RuntimeLogExportFormat,
} from "./log-labels";
import {
  formatTaskBadge,
  isRuntimeLogRange,
  levelMeta,
  moduleLabels,
  rangeLabels,
  runtimeLogLevels,
  runtimeLogModules,
  runtimeLogRanges,
} from "./log-labels";
import type { StreamStatus } from "./queries";
import {
  maxKeywordLength,
  maxLiveEntries,
  useClearRuntimeLogs,
  useRuntimeLogFilters,
  useRuntimeLogs,
  useRuntimeLogStream,
  useRuntimeLogTasks,
} from "./queries";
import { RuntimeLogsTable } from "./runtime-logs-table";

type SelectItem = { id: string; label: string };

type NoticeState = {
  status: "success" | "warning" | "danger";
  message: string;
} | null;

const levelOptions: SelectItem[] = [
  { id: "all", label: "全部" },
  ...runtimeLogLevels.map((level) => ({
    id: level,
    label: levelMeta[level].label,
  })),
];

const moduleOptions: SelectItem[] = [
  { id: "all", label: "全部" },
  ...runtimeLogModules.map((module) => ({
    id: module,
    label: moduleLabels[module],
  })),
];

const rangeOptions: SelectItem[] = runtimeLogRanges.map((range) => ({
  id: range,
  label: rangeLabels[range],
}));

function FilterSelect({
  label,
  value,
  options,
  onChange,
  leadingIcon,
}: {
  label: string;
  value: string;
  options: SelectItem[];
  onChange: (value: string) => void;
  leadingIcon?: "calendar";
}) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      {leadingIcon !== "calendar" && (
        <span
          aria-hidden="true"
          className="shrink-0 text-sm font-medium text-muted"
        >
          {label}
        </span>
      )}
      <div className="min-w-0 flex-1">
        <Select
          aria-label={label}
          fullWidth
          variant="secondary"
          value={value}
          onChange={(key) => {
            if (typeof key === "string") onChange(key);
          }}
        >
          <Select.Trigger>
            <span className="flex min-w-0 items-center gap-2">
              {leadingIcon === "calendar" && (
                <Calendar
                  aria-hidden="true"
                  className="size-4 shrink-0 text-muted"
                />
              )}
              <Select.Value />
            </span>
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox items={options}>
              {(option) => (
                <ListBox.Item id={option.id} textValue={option.label}>
                  {option.label}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              )}
            </ListBox>
          </Select.Popover>
        </Select>
      </div>
    </div>
  );
}

function StreamBadge({
  status,
  pausedByPage,
}: {
  status: StreamStatus;
  pausedByPage: boolean;
}) {
  if (pausedByPage) {
    return (
      <Chip size="sm" variant="soft" color="default">
        <Chip.Label>历史分页已暂停实时追加</Chip.Label>
      </Chip>
    );
  }
  if (status === "connected") {
    return (
      <Chip size="sm" variant="soft" color="success">
        <span
          aria-hidden="true"
          className="inline-block size-1.5 rounded-full bg-current"
        />
        <Chip.Label>实时同步中</Chip.Label>
      </Chip>
    );
  }
  if (status === "reconnecting") {
    return (
      <Chip size="sm" variant="soft" color="warning">
        <Chip.Label>实时流重连中</Chip.Label>
      </Chip>
    );
  }
  if (status === "connecting") {
    return (
      <Chip size="sm" variant="soft" color="default">
        <Chip.Label>连接实时流…</Chip.Label>
      </Chip>
    );
  }
  return null;
}

export function RuntimeLogsPage() {
  const queryState = useRuntimeLogFilters();
  const { filters, cursor } = queryState;
  const logsQuery = useRuntimeLogs(filters);
  const tasksQuery = useRuntimeLogTasks();
  const clearMutation = useClearRuntimeLogs();

  const [autoScroll, setAutoScroll] = useState(true);
  const [highlightErrors, setHighlightErrors] = useState(true);
  const [clearDialogOpen, setClearDialogOpen] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [notice, setNotice] = useState<NoticeState>(null);
  const [searchDraftState, setSearchDraftState] = useState<{
    source: string;
    value: string;
  }>({ source: queryState.keyword, value: queryState.keyword });
  const searchDraft =
    searchDraftState.source === queryState.keyword
      ? searchDraftState.value
      : queryState.keyword;
  const [liveBuffer, setLiveBuffer] = useState<{
    scopeKey: string;
    items: RuntimeLogEntry[];
  }>({ scopeKey: "", items: [] });

  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const lastScrolledIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (searchDraft === queryState.keyword) return;
    const timer = window.setTimeout(() => {
      queryState.setKeyword(searchDraft);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchDraft, queryState]);

  const filterScopeKey = useMemo(
    () =>
      JSON.stringify({
        range: filters.range,
        level: filters.level ?? "",
        module: filters.module ?? "",
        job_id: filters.job_id ?? "",
        q: filters.q ?? "",
        cursor: filters.cursor ?? "",
      }),
    [filters],
  );

  const isFirstPage = cursor === "";
  const streamEnabled = autoScroll && isFirstPage;

  const handleStreamEntry = useCallback(
    (entry: RuntimeLogEntry) => {
      setLiveBuffer((current) => {
        const baseItems =
          current.scopeKey === filterScopeKey ? current.items : [];
        if (baseItems.some((item) => item.id === entry.id)) return current;
        return {
          scopeKey: filterScopeKey,
          items: [entry, ...baseItems].slice(0, maxLiveEntries),
        };
      });
    },
    [filterScopeKey],
  );

  const handleStreamCleared = useCallback(() => {
    setLiveBuffer({ scopeKey: filterScopeKey, items: [] });
  }, [filterScopeKey]);

  const streamStatus = useRuntimeLogStream({
    enabled: streamEnabled,
    filters,
    onEntry: handleStreamEntry,
    onCleared: handleStreamCleared,
  });

  const mergedEntries = useMemo(() => {
    const serverItems = logsQuery.data?.items ?? [];
    if (!isFirstPage || liveBuffer.scopeKey !== filterScopeKey) {
      return serverItems;
    }
    if (liveBuffer.items.length === 0) return serverItems;
    const seen = new Set<string>();
    const combined: RuntimeLogEntry[] = [];
    for (const item of liveBuffer.items) {
      if (seen.has(item.id)) continue;
      seen.add(item.id);
      combined.push(item);
    }
    for (const item of serverItems) {
      if (seen.has(item.id)) continue;
      seen.add(item.id);
      combined.push(item);
    }
    return combined;
  }, [logsQuery.data?.items, isFirstPage, liveBuffer, filterScopeKey]);

  const newestLiveId =
    liveBuffer.scopeKey === filterScopeKey
      ? (liveBuffer.items[0]?.id ?? null)
      : null;

  useEffect(() => {
    if (!streamEnabled || !newestLiveId) return;
    if (lastScrolledIdRef.current === newestLiveId) return;
    lastScrolledIdRef.current = newestLiveId;
    const container = scrollContainerRef.current;
    if (container && typeof container.scrollTo === "function") {
      container.scrollTo({ top: 0, behavior: "smooth" });
    } else if (container) {
      container.scrollTop = 0;
    }
  }, [streamEnabled, newestLiveId]);

  const taskOptions = useMemo<SelectItem[]>(() => {
    const options: SelectItem[] = [{ id: "all", label: "全部" }];
    const seen = new Set<string>(["all"]);
    for (const task of tasksQuery.data?.items ?? []) {
      if (seen.has(task.id)) continue;
      seen.add(task.id);
      const typeLabel = taskTypes[task.type].label;
      const suffix = task.library_name
        ? `${typeLabel} · ${task.library_name}`
        : typeLabel;
      options.push({
        id: task.id,
        label: `${formatTaskBadge(task.id)} ${suffix}`,
      });
    }
    if (queryState.jobId && !seen.has(queryState.jobId)) {
      options.push({
        id: queryState.jobId,
        label: formatTaskBadge(queryState.jobId),
      });
    }
    return options;
  }, [tasksQuery.data?.items, queryState.jobId]);

  const hasActiveFilters =
    queryState.range !== "24h" ||
    queryState.level !== "all" ||
    queryState.module !== "all" ||
    queryState.jobId !== "" ||
    queryState.keyword.trim() !== "";

  async function handleExport(format: RuntimeLogExportFormat) {
    setExporting(true);
    setNotice(null);
    try {
      const result = await exportRuntimeLogs(filters, format);
      if (result.truncated) {
        setNotice({
          status: "warning",
          message: `已下载 ${result.filename}。导出已达到单次 10,000 条上限，仅包含最新匹配的 10,000 条日志。`,
        });
      } else {
        setNotice({
          status: "success",
          message: `已开始下载 ${result.filename}。`,
        });
      }
    } catch (error) {
      setNotice({
        status: "danger",
        message:
          error instanceof ApiError
            ? `${error.message}${error.requestId ? `（请求 ID ${error.requestId}）` : ""}`
            : "下载日志失败，请稍后重试。",
      });
    } finally {
      setExporting(false);
    }
  }

  function handleConfirmClear(scope: ClearRuntimeLogsScope) {
    clearMutation.mutate(scope, {
      onSuccess: (result) => {
        setClearDialogOpen(false);
        setLiveBuffer({ scopeKey: filterScopeKey, items: [] });
        if (cursor) queryState.setCursor("");
        setNotice({
          status: "success",
          message: `已清理 ${result.deleted_count} 条运行日志。`,
        });
      },
    });
  }

  function handleRefresh() {
    setNotice(null);
    setLiveBuffer({ scopeKey: filterScopeKey, items: [] });
    void logsQuery.refetch();
  }

  return (
    <main className="mx-auto grid w-full min-w-0 max-w-[1600px] gap-4 px-3 pt-4 pb-9 sm:px-5 sm:pt-5 sm:pb-12">
      <PageBanner
        title="运行日志"
        description="查看系统运行日志，支持按时间、级别、模块、关键词筛选，并可下载与清理日志。"
      />

      <Card>
        <Card.Content>
          <div className="grid gap-3">
            <div className="grid grid-cols-1 items-center gap-2.5 sm:grid-cols-2 lg:grid-cols-[minmax(9.5rem,auto)_minmax(8.5rem,auto)_minmax(8.5rem,auto)_minmax(8.5rem,auto)_minmax(17rem,1fr)]">
              <FilterSelect
                label="时间范围"
                leadingIcon="calendar"
                value={queryState.range}
                options={rangeOptions}
                onChange={(value) => {
                  if (isRuntimeLogRange(value)) queryState.setRange(value);
                }}
              />
              <FilterSelect
                label="日志级别"
                value={queryState.level}
                options={levelOptions}
                onChange={queryState.setLevel}
              />
              <FilterSelect
                label="模块"
                value={queryState.module}
                options={moduleOptions}
                onChange={queryState.setModule}
              />
              <FilterSelect
                label="任务"
                value={queryState.jobId || "all"}
                options={taskOptions}
                onChange={queryState.setJobId}
              />
              <div className="sm:col-span-2 lg:col-span-1">
                <SearchField
                  aria-label="搜索关键词"
                  fullWidth
                  value={searchDraft}
                  onChange={(value) =>
                    setSearchDraftState({ source: queryState.keyword, value })
                  }
                >
                  <SearchField.Group>
                    <SearchField.SearchIcon />
                    <SearchField.Input
                      placeholder="搜索关键词（文件名、路径、错误信息）"
                      maxLength={maxKeywordLength}
                    />
                    <SearchField.ClearButton />
                  </SearchField.Group>
                </SearchField>
              </div>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-2">
                <StreamBadge
                  status={streamStatus}
                  pausedByPage={autoScroll && !isFirstPage}
                />
                {hasActiveFilters && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => {
                      setSearchDraftState({ source: "", value: "" });
                      queryState.reset();
                    }}
                  >
                    重置筛选
                  </Button>
                )}
              </div>

              <div className="flex flex-wrap items-center justify-end gap-3">
                <Switch isSelected={autoScroll} onChange={setAutoScroll}>
                  <Switch.Content className="flex items-center gap-2">
                    <Label className="text-sm font-medium">自动滚动</Label>
                    <Switch.Control>
                      <Switch.Thumb />
                    </Switch.Control>
                  </Switch.Content>
                </Switch>

                <Separator
                  orientation="vertical"
                  className="hidden h-4 sm:block"
                />

                <Checkbox
                  variant="secondary"
                  isSelected={highlightErrors}
                  onChange={setHighlightErrors}
                >
                  <Checkbox.Content>
                    <Checkbox.Control>
                      <Checkbox.Indicator />
                    </Checkbox.Control>
                    <Label className="text-sm font-medium">高亮错误</Label>
                  </Checkbox.Content>
                </Checkbox>

                <Separator
                  orientation="vertical"
                  className="hidden h-4 sm:block"
                />

                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    isDisabled={logsQuery.isFetching}
                    onPress={handleRefresh}
                  >
                    <RefreshCw
                      aria-hidden="true"
                      className={`size-4${logsQuery.isFetching ? " animate-spin" : ""}`}
                    />
                    刷新
                  </Button>

                  <Dropdown>
                    <Button size="sm" variant="outline" isDisabled={exporting}>
                      <Download aria-hidden="true" className="size-4" />
                      {exporting ? "正在下载…" : "下载日志"}
                      <ChevronDown aria-hidden="true" className="size-4" />
                    </Button>
                    <Dropdown.Popover>
                      <Dropdown.Menu
                        aria-label="下载日志格式"
                        onAction={(key) => {
                          if (key === "log" || key === "jsonl") {
                            void handleExport(key);
                          }
                        }}
                      >
                        <Dropdown.Item
                          id="log"
                          textValue="下载当前筛选日志（纯文本 .log）"
                        >
                          <Label>下载当前筛选日志（纯文本 .log）</Label>
                        </Dropdown.Item>
                        <Dropdown.Item
                          id="jsonl"
                          textValue="下载当前筛选日志（结构化 .jsonl）"
                        >
                          <Label>下载当前筛选日志（结构化 .jsonl）</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>

                  <Button
                    size="sm"
                    variant="danger-soft"
                    onPress={() => {
                      clearMutation.reset();
                      setClearDialogOpen(true);
                    }}
                  >
                    <Trash2 aria-hidden="true" className="size-4" />
                    清理日志
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </Card.Content>
      </Card>

      {notice && (
        <Alert
          role={notice.status === "danger" ? "alert" : "status"}
          status={notice.status}
        >
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Description className="wrap-anywhere">
              {notice.message}
            </Alert.Description>
          </Alert.Content>
        </Alert>
      )}

      <Card>
        <Card.Content>
          <div className="grid min-w-0 gap-3">
            {logsQuery.isError && (
              <Alert role="alert" status="danger">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Description className="wrap-anywhere">
                    {logsQuery.error instanceof ApiError
                      ? logsQuery.error.message
                      : "无法加载运行日志，请稍后重试。"}
                    {logsQuery.error instanceof ApiError &&
                    logsQuery.error.requestId
                      ? `（请求 ID ${logsQuery.error.requestId}）`
                      : ""}
                  </Alert.Description>
                  <div>
                    <Button
                      size="sm"
                      variant="secondary"
                      onPress={() => void logsQuery.refetch()}
                    >
                      重新加载
                    </Button>
                  </div>
                </Alert.Content>
              </Alert>
            )}

            {logsQuery.isPending ? (
              <div
                className="flex min-h-40 items-center justify-center gap-2 text-sm text-muted"
                role="status"
              >
                <Spinner size="sm" aria-hidden="true" />
                正在加载运行日志…
              </div>
            ) : mergedEntries.length > 0 ? (
              <RuntimeLogsTable
                entries={mergedEntries}
                highlightErrors={highlightErrors}
                scrollContainerRef={scrollContainerRef}
              />
            ) : !logsQuery.isError ? (
              <EmptyState>
                <div className="grid min-h-40 place-content-center justify-items-center gap-2 px-4 py-6 text-center">
                  <FileText className="size-6 text-muted" aria-hidden="true" />
                  <strong className="text-foreground">
                    {hasActiveFilters
                      ? "没有符合条件的运行日志"
                      : "当前时间范围内暂无运行日志"}
                  </strong>
                  <p className="text-sm text-muted">
                    {hasActiveFilters
                      ? "可调整时间范围、级别、模块或关键词后重新查看。"
                      : "系统执行同步、扫描、STRM 生成或调度任务时将自动记录日志。"}
                  </p>
                </div>
              </EmptyState>
            ) : null}

            {(cursor || logsQuery.data?.next_cursor) && (
              <div className="flex justify-end">
                <Pagination aria-label="运行日志分页" size="sm">
                  <Pagination.Content>
                    {cursor && (
                      <Pagination.Item>
                        <Pagination.Previous
                          onPress={() => queryState.setCursor("")}
                        >
                          <Pagination.PreviousIcon />
                          返回第一页
                        </Pagination.Previous>
                      </Pagination.Item>
                    )}
                    {logsQuery.data?.next_cursor && (
                      <Pagination.Item>
                        <Pagination.Next
                          onPress={() => {
                            if (logsQuery.data?.next_cursor) {
                              queryState.setCursor(logsQuery.data.next_cursor);
                            }
                          }}
                        >
                          下一页
                          <Pagination.NextIcon />
                        </Pagination.Next>
                      </Pagination.Item>
                    )}
                  </Pagination.Content>
                </Pagination>
              </div>
            )}
          </div>
        </Card.Content>
      </Card>

      <ClearLogsDialog
        open={clearDialogOpen}
        pending={clearMutation.isPending}
        error={clearMutation.error}
        onOpenChange={setClearDialogOpen}
        onConfirm={handleConfirmClear}
      />
    </main>
  );
}
