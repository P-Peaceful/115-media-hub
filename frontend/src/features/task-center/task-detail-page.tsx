import {
  Button,
  Chip,
  Dropdown,
  Label,
  Link,
  Table,
  Tabs,
} from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import {
  ChartNoAxesColumn,
  ChevronDown,
  CircleCheck,
  CircleMinus,
  CircleX,
  Clock3,
  FileWarning,
  Film,
  Gauge,
  Info,
  ListVideo,
  ScrollText,
} from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router";

import type { Task } from "./api";
import { getTask, listFailures, listLogs } from "./api";
import { taskKeys, useTaskAction, useTaskSearch } from "./queries";
import type { TaskAction } from "./task-kinds";
import { taskActions, taskSources, taskTypes } from "./task-kinds";
import {
  ConfirmAction,
  EmptyState,
  formatCount,
  formatDate,
  formatDuration,
  formatSpeed,
  headingClass,
  Panel,
  recordLabelClass,
  StatusChip,
  TaskBackLink,
  TaskError,
  TaskLoading,
  taskPageClass,
  TaskPagination,
  TaskProgress,
  titleClass,
  toolbarClass,
} from "./task-ui";

function TaskRecords({
  task,
  tab,
  cursor,
  onChange,
}: {
  task: Task;
  tab: "failures" | "logs";
  cursor: string;
  onChange: (cursor: string) => void;
}) {
  const params = new URLSearchParams();
  if (cursor) params.set("cursor", cursor);
  const failures = useQuery({
    queryKey: [...taskKeys.detail(task.id), "failures", cursor],
    queryFn: ({ signal }) => listFailures(task.id, params, signal),
    enabled: tab === "failures",
    retry: false,
  });
  const logs = useQuery({
    queryKey: [...taskKeys.detail(task.id), "logs", cursor],
    queryFn: ({ signal }) => listLogs(task.id, params, signal),
    enabled: tab === "logs",
    retry: false,
  });
  const query = tab === "logs" ? logs : failures;
  if (query.isError)
    return (
      <TaskError
        error={query.error}
        retry={() => {
          void query.refetch();
        }}
      />
    );
  if (query.isPending) return <TaskLoading />;
  if (!query.data.items.length)
    return tab === "logs" ? (
      <EmptyState icon={ScrollText} title="暂无关联日志" />
    ) : (
      <EmptyState icon={FileWarning} title="暂无失败文件" />
    );

  return (
    <>
      {tab === "failures" && failures.data && (
        <Table>
          <Table.ScrollContainer>
            <Table.Content aria-label="失败文件" className="min-w-72">
              <Table.Header>
                <Table.Column isRowHeader>文件路径</Table.Column>
                <Table.Column>错误原因</Table.Column>
              </Table.Header>
              <Table.Body items={failures.data.items}>
                {(failure) => (
                  <Table.Row id={failure.id}>
                    <Table.Cell className="wrap-anywhere">
                      {failure.path}
                    </Table.Cell>
                    <Table.Cell>
                      <Chip color="danger" variant="soft" size="sm">
                        <Chip.Label>{failure.reason}</Chip.Label>
                      </Chip>
                    </Table.Cell>
                  </Table.Row>
                )}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
      )}
      {tab === "logs" && logs.data && (
        <Table>
          <Table.ScrollContainer>
            <Table.Content aria-label="关联日志" className="min-w-80">
              <Table.Header>
                <Table.Column>时间</Table.Column>
                <Table.Column>级别</Table.Column>
                <Table.Column isRowHeader>内容</Table.Column>
              </Table.Header>
              <Table.Body items={logs.data.items}>
                {(log) => (
                  <Table.Row id={log.id}>
                    <Table.Cell>{formatDate(log.created_at)}</Table.Cell>
                    <Table.Cell>{log.level}</Table.Cell>
                    <Table.Cell className="wrap-anywhere">
                      {log.message}
                    </Table.Cell>
                  </Table.Row>
                )}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
      )}
      <TaskPagination
        cursor={cursor}
        next={query.data.next_cursor}
        onChange={onChange}
      />
    </>
  );
}

function TaskDetail({ task }: { task: Task }) {
  const { search, cursor, change } = useTaskSearch();
  const tab = search.get("tab") === "logs" ? "logs" : "failures";
  const from = search.get("from");
  const backHref = from?.startsWith("?") ? `/tasks${from}` : "/tasks";
  const navigate = useNavigate();
  const actionMutation = useTaskAction();
  const [confirmCancel, setConfirmCancel] = useState(false);
  const source = taskSources[task.source];
  const SourceIcon = source.icon;
  const retries = task.actions.filter(
    (action) => action === "retry" || action === "retry_failed",
  );

  function act(action: TaskAction) {
    if (action === "cancel") {
      actionMutation.reset();
      setConfirmCancel(true);
      return;
    }
    actionMutation.mutate(
      { id: task.id, action },
      {
        onSuccess: (next) => {
          if (next.id !== task.id)
            void navigate(`/tasks/${encodeURIComponent(next.id)}`);
        },
      },
    );
  }

  return (
    <main className={taskPageClass}>
      <header className={headingClass}>
        <TaskBackLink label="返回任务中心" href={backHref} />
        <div className="min-w-0 flex-[1_1_15rem]">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className={titleClass}>{taskTypes[task.type].label}任务</h1>
            <StatusChip task={task} />
          </div>
          <div className="flex flex-wrap gap-x-4 gap-y-1 wrap-anywhere text-sm text-muted">
            <span>任务 ID {task.id}</span>
            <span>创建于 {formatDate(task.created_at)}</span>
            {task.duration_ms !== null && (
              <span>耗时 {formatDuration(task.duration_ms)}</span>
            )}
          </div>
        </div>
        <div className={toolbarClass}>
          <Button
            variant="secondary"
            size="sm"
            onPress={() => change("tab", "logs")}
          >
            <ScrollText aria-hidden="true" />
            查看日志
          </Button>
          {task.actions
            .filter((action) => action !== "retry" && action !== "retry_failed")
            .map((action) => {
              const { icon: Icon, label } = taskActions[action];
              return (
                <Button
                  key={action}
                  size="sm"
                  variant={action === "cancel" ? "danger-soft" : "secondary"}
                  isDisabled={actionMutation.isPending}
                  onPress={() => act(action)}
                >
                  <Icon aria-hidden="true" />
                  {label}
                </Button>
              );
            })}
          {retries.length > 0 && (
            <Dropdown>
              <Button size="sm" isDisabled={actionMutation.isPending}>
                失败重试 <ChevronDown aria-hidden="true" />
              </Button>
              <Dropdown.Popover>
                <Dropdown.Menu
                  aria-label="失败重试选项"
                  onAction={(action) => {
                    if (action === "retry" || action === "retry_failed")
                      act(action);
                  }}
                >
                  {retries.map((action) => {
                    const { icon: Icon, label } = taskActions[action];
                    return (
                      <Dropdown.Item key={action} id={action} textValue={label}>
                        <Icon aria-hidden="true" />
                        <Label>{label}</Label>
                      </Dropdown.Item>
                    );
                  })}
                </Dropdown.Menu>
              </Dropdown.Popover>
            </Dropdown>
          )}
        </div>
      </header>
      {!confirmCancel && <TaskError error={actionMutation.error} />}
      <div className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-[minmax(12rem,0.9fr)_minmax(0,1fr)_minmax(0,1fr)]">
        <Panel
          title="任务信息"
          icon={Info}
          className="md:col-span-2 xl:col-span-1 xl:row-span-2"
        >
          <dl className="grid gap-3 wrap-anywhere md:grid-cols-2 xl:grid-cols-1">
            <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-start gap-2">
              <dt className="flex items-center gap-1 text-sm text-muted">
                <SourceIcon aria-hidden="true" className="size-4" />
                来源
              </dt>
              <dd className="font-medium">{source.label}</dd>
            </div>
            <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-start gap-2">
              <dt className="flex items-center gap-1 text-sm text-muted">
                <Film aria-hidden="true" className="size-4" />
                所属媒体库
              </dt>
              <dd className="font-medium">
                {task.library_name || task.library_id}
              </dd>
            </div>
            {task.schedule_id && (
              <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-start gap-2">
                <dt className="flex items-center gap-1 text-sm text-muted">
                  所属计划
                </dt>
                <dd className="font-medium">
                  <Link
                    href={`/tasks/schedules/${encodeURIComponent(task.schedule_id)}/edit`}
                  >
                    查看定时计划
                  </Link>
                </dd>
              </div>
            )}
            {task.parent_id && (
              <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-start gap-2">
                <dt className="flex items-center gap-1 text-sm text-muted">
                  父任务
                </dt>
                <dd className="font-medium">
                  <Link href={`/tasks/${encodeURIComponent(task.parent_id)}`}>
                    查看原任务
                  </Link>
                </dd>
              </div>
            )}
          </dl>
        </Panel>
        <Panel title="结果统计" icon={ChartNoAxesColumn}>
          <dl className="grid grid-cols-2 gap-3 text-center sm:grid-cols-4">
            <div>
              <dt className="text-xs text-muted">总文件数</dt>
              <dd className="flex flex-wrap items-center justify-center gap-1 font-semibold tabular-nums">
                {formatCount(task.total_items)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted">成功</dt>
              <dd className="flex flex-wrap items-center justify-center gap-1 font-semibold tabular-nums">
                <CircleCheck
                  className="size-4 text-success"
                  aria-hidden="true"
                />
                {formatCount(task.succeeded_items)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted">失败</dt>
              <dd className="flex flex-wrap items-center justify-center gap-1 font-semibold tabular-nums">
                <CircleX className="size-4 text-danger" aria-hidden="true" />
                {formatCount(task.failed_items)}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted">跳过</dt>
              <dd className="flex flex-wrap items-center justify-center gap-1 font-semibold tabular-nums">
                <CircleMinus className="size-4 text-muted" aria-hidden="true" />
                {formatCount(task.skipped_items)}
              </dd>
            </div>
          </dl>
        </Panel>
        <Panel title="执行进度" icon={Gauge}>
          <div className="grid gap-3 text-sm">
            <TaskProgress value={task.progress_percent} />
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-muted">
              <span>
                <Clock3 aria-hidden="true" className="inline size-4" />
                耗时 <strong>{formatDuration(task.duration_ms)}</strong>
              </span>
              <span>
                <Gauge aria-hidden="true" className="inline size-4" />
                速度 <strong>{formatSpeed(task.speed_bytes_per_second)}</strong>
              </span>
            </div>
            {task.current_file && (
              <p className={recordLabelClass} title={task.current_file}>
                {task.current_file}
              </p>
            )}
          </div>
        </Panel>
        <Panel title="执行记录" icon={ListVideo} className="md:col-span-2">
          <Tabs
            selectedKey={tab}
            onSelectionChange={(key) =>
              change("tab", key === "logs" ? "logs" : "")
            }
          >
            <Tabs.ListContainer>
              <Tabs.List aria-label="记录类型">
                <Tabs.Tab id="failures">
                  失败文件（{formatCount(task.failed_items)}）
                  <Tabs.Indicator />
                </Tabs.Tab>
                <Tabs.Tab id="logs">
                  关联日志
                  <Tabs.Indicator />
                </Tabs.Tab>
              </Tabs.List>
            </Tabs.ListContainer>
            <Tabs.Panel id="failures">
              <TaskRecords
                task={task}
                tab="failures"
                cursor={cursor}
                onChange={(value) => change("cursor", value, false)}
              />
            </Tabs.Panel>
            <Tabs.Panel id="logs">
              <TaskRecords
                task={task}
                tab="logs"
                cursor={cursor}
                onChange={(value) => change("cursor", value, false)}
              />
            </Tabs.Panel>
          </Tabs>
        </Panel>
      </div>
      {confirmCancel && (
        <ConfirmAction
          open
          onOpenChange={setConfirmCancel}
          title="取消任务"
          body="确认取消当前任务？"
          label="确认取消"
          pending={actionMutation.isPending}
          error={actionMutation.error}
          onConfirm={() =>
            actionMutation.mutate(
              { id: task.id, action: "cancel" },
              { onSuccess: () => setConfirmCancel(false) },
            )
          }
        />
      )}
    </main>
  );
}

export function TaskDetailPage() {
  const { id = "" } = useParams();
  const query = useQuery({
    queryKey: taskKeys.detail(id),
    queryFn: ({ signal }) => getTask(id, signal),
    retry: false,
  });
  if (query.isSuccess)
    return <TaskDetail key={query.data.id} task={query.data} />;
  return (
    <main className={taskPageClass}>
      <header className={headingClass}>
        <TaskBackLink label="返回任务中心" href="/tasks" />
        <h1 className={titleClass}>任务详情</h1>
      </header>
      {query.isError ? (
        <TaskError
          error={query.error}
          retry={() => {
            void query.refetch();
          }}
        />
      ) : (
        <TaskLoading>正在加载任务…</TaskLoading>
      )}
    </main>
  );
}
