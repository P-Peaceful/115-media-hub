import {
  Button,
  Link,
  Modal,
  Table,
  ToggleButton,
  ToggleButtonGroup,
} from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import {
  CalendarClock,
  ChartNoAxesColumn,
  LayoutGrid,
  ListChecks,
  ListVideo,
  Plus,
  RefreshCw,
} from "lucide-react";
import { useState } from "react";
import { useLocation, useNavigate } from "react-router";

import { PageBanner } from "@/components/page-banner";
import type { Library } from "@/features/media-library/api";

import type { Task, TaskTypeCapability } from "./api";
import { listTasks } from "./api";
import {
  taskKeys,
  useSubmitTask,
  useTaskAction,
  useTaskCapabilities,
  useTaskLibraries,
  useTaskSearch,
} from "./queries";
import type { TaskType } from "./task-kinds";
import {
  hasKey,
  keys,
  taskActions,
  taskStatuses,
  taskTypes,
} from "./task-kinds";
import {
  ConfirmAction,
  emptyMetric,
  EmptyState,
  formatCount,
  formatDuration,
  formatSpeed,
  Panel,
  recordLabelClass,
  StatusChip,
  TaskButtonLink,
  TaskError,
  TaskLoading,
  taskPageClass,
  TaskPagination,
  TaskProgress,
  TaskSelect,
  toolbarClass,
  TypeChip,
} from "./task-ui";

const typeOptions = keys(taskTypes).map((id) => ({
  id,
  label: taskTypes[id].label,
}));
const manualTypeOptions = typeOptions.filter(({ id }) => id !== "strm");
const statusOptions = keys(taskStatuses).map((id) => ({
  id,
  label: taskStatuses[id].label,
}));

function ManualTaskDialog({
  capabilities,
  libraries,
}: {
  capabilities: TaskTypeCapability[];
  libraries: Library[];
}) {
  const [open, setOpen] = useState(false);
  const [type, setType] = useState<Exclude<TaskType, "strm"> | "">("");
  const [libraryId, setLibraryId] = useState("");
  const navigate = useNavigate();
  const capability = capabilities.find((entry) => entry.type === type);
  const mutation = useSubmitTask();
  const options = manualTypeOptions.map((option) => ({
    ...option,
    label: capabilities.some((cap) => cap.type === option.id && cap.available)
      ? option.label
      : `${option.label}（暂不可用）`,
  }));
  const libraryOptions = libraries
    .filter((library) => library.enabled)
    .map((library) => ({ id: library.id, label: library.name }));

  return (
    <Modal
      isOpen={open}
      onOpenChange={(next) => {
        if (mutation.isPending) return;
        if (next) mutation.reset();
        setOpen(next);
      }}
    >
      <Button
        size="sm"
        onPress={() => {
          mutation.reset();
          setOpen(true);
        }}
      >
        <Plus aria-hidden="true" />
        提交任务
      </Button>
      <Modal.Backdrop>
        <Modal.Container>
          <Modal.Dialog>
            <Modal.Header>
              <Modal.Heading>提交任务</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <TaskSelect
                label="任务类型"
                value={type}
                options={options}
                onChange={(value) => {
                  if (hasKey(taskTypes, value) && value !== "strm") {
                    setType(value);
                  }
                }}
              />
              {capability && !capability.available && (
                <p className="text-muted">
                  {capability.reason ?? "此类型暂不可用。"}
                </p>
              )}
              <TaskSelect
                label="目标媒体库"
                value={libraryId}
                options={libraryOptions}
                onChange={setLibraryId}
                disabled={libraryOptions.length === 0}
              />
              {libraryOptions.length === 0 && (
                <p className="text-muted">暂无已启用的媒体库。</p>
              )}
              <TaskError error={mutation.error} />
            </Modal.Body>
            <Modal.Footer>
              <Button
                variant="secondary"
                isDisabled={mutation.isPending}
                onPress={() => setOpen(false)}
              >
                取消
              </Button>
              <Button
                isDisabled={
                  !type ||
                  !capability?.available ||
                  !libraryOptions.some((library) => library.id === libraryId)
                }
                isPending={mutation.isPending}
                onPress={() => {
                  if (type) {
                    mutation.mutate(
                      { type, libraryId },
                      {
                        onSuccess: (task) => {
                          setOpen(false);
                          void navigate(
                            `/tasks/${encodeURIComponent(task.id)}`,
                          );
                        },
                      },
                    );
                  }
                }}
              >
                提交
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}

function TaskTable({ items }: { items: Task[] }) {
  const location = useLocation();
  const mutation = useTaskAction();
  const [canceling, setCanceling] = useState<Task | null>(null);

  return (
    <>
      {!canceling && <TaskError error={mutation.error} />}
      <Table>
        <Table.ScrollContainer>
          <Table.Content aria-label="任务列表" className="min-w-[56rem]">
            <Table.Header>
              <Table.Column isRowHeader>任务名称</Table.Column>
              <Table.Column>类型</Table.Column>
              <Table.Column>所属媒体库</Table.Column>
              <Table.Column>状态</Table.Column>
              <Table.Column>进度</Table.Column>
              <Table.Column>速度</Table.Column>
              <Table.Column>耗时</Table.Column>
              <Table.Column>当前处理文件</Table.Column>
              <Table.Column>操作</Table.Column>
            </Table.Header>
            <Table.Body items={items}>
              {(task) => {
                const kind = taskTypes[task.type];
                const Icon = kind.icon;
                return (
                  <Table.Row id={task.id}>
                    <Table.Cell>
                      <Link
                        href={`/tasks/${encodeURIComponent(task.id)}${location.search ? `?from=${encodeURIComponent(location.search)}` : ""}`}
                      >
                        <Icon aria-hidden="true" className="size-4 shrink-0" />
                        <span>
                          {kind.label} · {task.library_name || task.library_id}
                        </span>
                      </Link>
                    </Table.Cell>
                    <Table.Cell>
                      <TypeChip type={task.type} />
                    </Table.Cell>
                    <Table.Cell>
                      <span
                        className={recordLabelClass}
                        title={task.library_name}
                      >
                        {task.library_name || task.library_id}
                      </span>
                    </Table.Cell>
                    <Table.Cell>
                      <StatusChip task={task} />
                    </Table.Cell>
                    <Table.Cell>
                      <TaskProgress value={task.progress_percent} />
                    </Table.Cell>
                    <Table.Cell>
                      {formatSpeed(task.speed_bytes_per_second)}
                    </Table.Cell>
                    <Table.Cell>{formatDuration(task.duration_ms)}</Table.Cell>
                    <Table.Cell>
                      <span
                        className={recordLabelClass}
                        title={task.current_file ?? undefined}
                      >
                        {task.current_file ?? emptyMetric}
                      </span>
                    </Table.Cell>
                    <Table.Cell>
                      <div className="flex items-center gap-1">
                        {task.actions
                          .filter(
                            (action) =>
                              action !== "retry" && action !== "retry_failed",
                          )
                          .map((action) => {
                            const { icon: ActionIcon, label } =
                              taskActions[action];
                            return (
                              <Button
                                key={action}
                                aria-label={label}
                                size="sm"
                                isIconOnly
                                variant={
                                  action === "cancel" ? "danger-soft" : "ghost"
                                }
                                isDisabled={mutation.isPending}
                                onPress={() => {
                                  if (action === "cancel") {
                                    mutation.reset();
                                    setCanceling(task);
                                  } else
                                    mutation.mutate({ id: task.id, action });
                                }}
                              >
                                <ActionIcon aria-hidden="true" />
                              </Button>
                            );
                          })}
                      </div>
                    </Table.Cell>
                  </Table.Row>
                );
              }}
            </Table.Body>
          </Table.Content>
        </Table.ScrollContainer>
      </Table>
      {canceling && (
        <ConfirmAction
          open
          onOpenChange={(open) => {
            if (!open) {
              setCanceling(null);
              mutation.reset();
            }
          }}
          title="取消任务"
          body={`确认取消「${taskTypes[canceling.type].label} · ${canceling.library_name || canceling.library_id}」？`}
          label="确认取消"
          pending={mutation.isPending}
          error={mutation.error}
          onConfirm={() =>
            mutation.mutate(
              { id: canceling.id, action: "cancel" },
              { onSuccess: () => setCanceling(null) },
            )
          }
        />
      )}
    </>
  );
}

export function TaskOverviewPage() {
  const { search, change, cursor } = useTaskSearch();
  const rawType = search.get("type");
  const rawStatus = search.get("status");
  const type = hasKey(taskTypes, rawType) ? rawType : "";
  const status = hasKey(taskStatuses, rawStatus) ? rawStatus : "";
  const library = search.get("library_id") ?? "";
  const params = new URLSearchParams();
  if (type) params.set("type", type);
  if (status) params.set("status", status);
  if (library) params.set("library_id", library);
  if (cursor) params.set("cursor", cursor);
  const query = useQuery({
    queryKey: taskKeys.list(params.toString()),
    queryFn: ({ signal }) => listTasks(params, signal),
    retry: false,
  });
  const capabilities = useTaskCapabilities();
  const libraries = useTaskLibraries();
  const manualCapabilities = capabilities.data?.filter(
    ({ type }) => type !== "strm",
  );
  const allCount = query.data
    ? keys(taskTypes).reduce(
        (sum, kind) => sum + (query.data.by_type[kind] ?? 0),
        0,
      )
    : undefined;
  const libraryOptions = [
    { id: "all", label: "全部媒体库" },
    ...(libraries.data ?? []).map(({ id, name }) => ({ id, label: name })),
  ];
  if (library && !libraryOptions.some(({ id }) => id === library)) {
    libraryOptions.push({ id: library, label: "媒体库已不可用" });
  }
  const filtered = Boolean(type || status || library);

  return (
    <main className={taskPageClass}>
      <PageBanner
        title="任务中心"
        description="查看和管理媒体库任务的进度、结果与历史记录。"
      />
      <div className="grid min-w-0 grid-cols-1 gap-4 xl:grid-cols-[1.25fr_1fr]">
        <Panel title="任务分类" icon={LayoutGrid}>
          <ToggleButtonGroup
            aria-label="任务分类"
            isDetached
            size="sm"
            selectionMode="single"
            selectedKeys={[type || "all"]}
            onSelectionChange={(selection) => {
              const [key] = selection;
              change("type", hasKey(taskTypes, key) ? key : "");
            }}
            className="flex w-full flex-wrap justify-start"
          >
            <ToggleButton id="all" aria-label={`全部 ${formatCount(allCount)}`}>
              <LayoutGrid aria-hidden="true" />
              全部
              <strong>{formatCount(allCount)}</strong>
            </ToggleButton>
            {typeOptions.map(({ id, label }) => {
              const { icon: Icon } = taskTypes[id];
              const count = formatCount(
                query.data ? (query.data.by_type[id] ?? 0) : undefined,
              );
              return (
                <ToggleButton key={id} id={id} aria-label={`${label} ${count}`}>
                  <Icon aria-hidden="true" />
                  {label}
                  <strong>{count}</strong>
                </ToggleButton>
              );
            })}
          </ToggleButtonGroup>
        </Panel>
        <Panel title="状态分组" icon={ChartNoAxesColumn}>
          <ToggleButtonGroup
            aria-label="状态分组"
            isDetached
            size="sm"
            selectionMode="single"
            selectedKeys={status ? [status] : []}
            onSelectionChange={(selection) => {
              const [key] = selection;
              change("status", hasKey(taskStatuses, key) ? key : "");
            }}
            className="flex w-full flex-wrap justify-start"
          >
            {statusOptions.map(({ id, label }) => {
              const { icon: Icon } = taskStatuses[id];
              const count = formatCount(
                query.data ? (query.data.by_status[id] ?? 0) : undefined,
              );
              return (
                <ToggleButton key={id} id={id} aria-label={`${label} ${count}`}>
                  <Icon aria-hidden="true" />
                  {label}
                  <strong>{count}</strong>
                </ToggleButton>
              );
            })}
          </ToggleButtonGroup>
        </Panel>
      </div>
      <Panel
        icon={ListChecks}
        title={
          <>
            任务列表{" "}
            {query.data && (
              <span className="text-muted">
                （{formatCount(query.data.total)}）
              </span>
            )}
          </>
        }
        actions={
          <div className={toolbarClass}>
            <TaskSelect
              label="筛选所属媒体库"
              value={library || "all"}
              options={libraryOptions}
              onChange={(value) =>
                change("library_id", value === "all" ? "" : value)
              }
            />
            <TaskSelect
              label="筛选任务状态"
              value={status || "all"}
              options={[{ id: "all", label: "全部状态" }, ...statusOptions]}
              onChange={(value) =>
                change("status", value === "all" ? "" : value)
              }
            />
            <Button
              size="sm"
              variant="secondary"
              isPending={query.isFetching && !query.isPending}
              onPress={() => {
                void query.refetch();
              }}
            >
              <RefreshCw aria-hidden="true" />
              刷新
            </Button>
            <TaskButtonLink href="/tasks/schedules">
              <CalendarClock aria-hidden="true" />
              定时计划
            </TaskButtonLink>
            {manualCapabilities?.some((item) => item.available) &&
              libraries.data && (
                <ManualTaskDialog
                  capabilities={manualCapabilities}
                  libraries={libraries.data}
                />
              )}
          </div>
        }
      >
        <TaskError
          error={capabilities.error || libraries.error}
          retry={() => {
            void capabilities.refetch();
            void libraries.refetch();
          }}
        />
        {query.isError ? (
          <TaskError
            error={query.error}
            retry={() => {
              void query.refetch();
            }}
          />
        ) : query.isPending ? (
          <TaskLoading />
        ) : query.data.items.length > 0 ? (
          <TaskTable items={query.data.items} />
        ) : (
          <EmptyState
            icon={ListVideo}
            title={filtered ? "没有符合条件的任务" : "暂无任务"}
          >
            {filtered ? "调整筛选条件后查看。" : "任务提交后会显示在这里。"}
          </EmptyState>
        )}
        <TaskPagination
          cursor={cursor}
          next={query.data?.next_cursor}
          onChange={(value) => change("cursor", value, false)}
        />
      </Panel>
    </main>
  );
}
