import { Button, Chip, Link, Table } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarClock,
  CalendarDays,
  CirclePause,
  CirclePlay,
  ClipboardList,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { useLocation } from "react-router";

import { PageBanner } from "@/components/page-banner";

import type { Schedule } from "./api";
import { deleteSchedule, listSchedules, setScheduleEnabled } from "./api";
import {
  scheduleKeys,
  useTaskCapabilities,
  useTaskCsrf,
  useTaskLibraries,
  useTaskSearch,
} from "./queries";
import { hasKey, keys, taskTypes } from "./task-kinds";
import {
  ConfirmAction,
  EmptyState,
  formatDate,
  Panel,
  recordLabelClass,
  TaskButtonLink,
  TaskError,
  TaskLoading,
  taskPageClass,
  TaskPagination,
  TaskSelect,
  toolbarClass,
  TypeChip,
} from "./task-ui";

const typeOptions = [
  { id: "all", label: "全部类型" },
  ...keys(taskTypes).map((id) => ({ id, label: taskTypes[id].label })),
];
const enabledOptions = [
  { id: "all", label: "全部状态" },
  { id: "true", label: "已启用" },
  { id: "false", label: "已停用" },
];

export function ScheduleListPage() {
  const { search, change, cursor } = useTaskSearch();
  const rawType = search.get("type");
  const type = hasKey(taskTypes, rawType) ? rawType : "";
  const library = search.get("library_id") ?? "";
  const rawEnabled = search.get("enabled");
  const enabled =
    rawEnabled === "true" || rawEnabled === "false" ? rawEnabled : "";
  const params = new URLSearchParams();
  if (type) params.set("type", type);
  if (library) params.set("library_id", library);
  if (enabled) params.set("enabled", enabled);
  if (cursor) params.set("cursor", cursor);
  const schedules = useQuery({
    queryKey: scheduleKeys.list(params.toString()),
    queryFn: ({ signal }) => listSchedules(params, signal),
    retry: false,
  });
  const libraries = useTaskLibraries();
  const capabilities = useTaskCapabilities();
  const libraryOptions = [
    { id: "all", label: "全部媒体库" },
    ...(libraries.data ?? []).map(({ id, name }) => ({ id, label: name })),
  ];
  if (library && !libraryOptions.some(({ id }) => id === library)) {
    libraryOptions.push({ id: library, label: "媒体库已不可用" });
  }

  const [deleting, setDeleting] = useState<Schedule | null>(null);
  const csrf = useTaskCsrf();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async ({
      schedule,
      action,
    }: {
      schedule: Schedule;
      action: "toggle" | "delete";
    }) => {
      if (action === "delete") {
        await deleteSchedule(csrf(), schedule.id);
      } else {
        await setScheduleEnabled(csrf(), schedule.id, !schedule.enabled);
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: scheduleKeys.all }),
  });
  const location = useLocation();
  const from = location.search
    ? `?from=${encodeURIComponent(location.search)}`
    : "";
  const filtered = Boolean(type || enabled || library);

  function changeFilter(key: string, value: string) {
    change(key, value === "all" ? "" : value);
  }

  return (
    <main className={taskPageClass}>
      <PageBanner
        title="定时计划"
        description="按计划执行扫描、STRM 生成、同步、检查、清理、刮削和回传任务。"
      />
      <Panel
        title="计划列表"
        icon={CalendarDays}
        actions={
          <div className={toolbarClass}>
            <TaskSelect
              label="筛选计划类型"
              value={type || "all"}
              options={typeOptions}
              onChange={(value) => changeFilter("type", value)}
            />
            <TaskSelect
              label="筛选所属媒体库"
              value={library || "all"}
              options={libraryOptions}
              onChange={(value) => changeFilter("library_id", value)}
            />
            <TaskSelect
              label="筛选启停状态"
              value={enabled || "all"}
              options={enabledOptions}
              onChange={(value) => changeFilter("enabled", value)}
            />
            <TaskButtonLink href="/tasks">
              <ClipboardList aria-hidden="true" />
              任务中心
            </TaskButtonLink>
            <TaskButtonLink
              variant="primary"
              href={`/tasks/schedules/new${from}`}
            >
              <Plus aria-hidden="true" />
              创建计划
            </TaskButtonLink>
          </div>
        }
      >
        {capabilities.data?.every((item) => !item.available) && (
          <p className="text-sm text-muted">
            可创建的计划类型将在对应业务模块接入后开放。
          </p>
        )}
        <TaskError
          error={libraries.error || capabilities.error}
          retry={() => {
            void libraries.refetch();
            void capabilities.refetch();
          }}
        />
        {!deleting && <TaskError error={mutation.error} />}
        {schedules.isError && (
          <TaskError
            error={schedules.error}
            retry={() => {
              void schedules.refetch();
            }}
          />
        )}
        {schedules.isPending && <TaskLoading />}
        {schedules.data?.items.length === 0 && (
          <EmptyState
            icon={CalendarClock}
            title={filtered ? "没有符合条件的计划" : "暂无定时计划"}
          />
        )}
        {schedules.data && schedules.data.items.length > 0 && (
          <Table>
            <Table.ScrollContainer>
              <Table.Content
                aria-label="定时计划列表"
                className="min-w-[42rem]"
              >
                <Table.Header>
                  <Table.Column isRowHeader>计划名称</Table.Column>
                  <Table.Column>任务类型</Table.Column>
                  <Table.Column>所属媒体库</Table.Column>
                  <Table.Column>启停状态</Table.Column>
                  <Table.Column>下次执行时间</Table.Column>
                  <Table.Column>操作</Table.Column>
                </Table.Header>
                <Table.Body items={schedules.data.items}>
                  {(schedule) => {
                    const typeInfo = taskTypes[schedule.type];
                    const Icon = typeInfo.icon;
                    const available =
                      capabilities.data?.some(
                        (entry) =>
                          entry.type === schedule.type && entry.available,
                      ) ?? false;
                    const libraryName =
                      schedule.library_id === null
                        ? "全部媒体库"
                        : (libraries.data?.find(
                            (item) => item.id === schedule.library_id,
                          )?.name ?? schedule.library_id);
                    const enabledLabel = schedule.enabled
                      ? "停用计划"
                      : "启用计划";
                    return (
                      <Table.Row id={schedule.id}>
                        <Table.Cell>
                          <Link
                            href={`/tasks/schedules/${encodeURIComponent(schedule.id)}/edit${from}`}
                          >
                            <Icon
                              aria-hidden="true"
                              className="size-4 shrink-0"
                            />
                            <span
                              className={recordLabelClass}
                              title={schedule.name}
                            >
                              {schedule.name}
                            </span>
                          </Link>
                        </Table.Cell>
                        <Table.Cell>
                          <TypeChip type={schedule.type} />
                          {capabilities.isSuccess && !available && (
                            <small className="block text-muted">暂不可用</small>
                          )}
                        </Table.Cell>
                        <Table.Cell>
                          <span
                            className={recordLabelClass}
                            title={libraryName}
                          >
                            {libraryName}
                          </span>
                        </Table.Cell>
                        <Table.Cell>
                          <Chip
                            size="sm"
                            variant="soft"
                            color={schedule.enabled ? "success" : "default"}
                          >
                            <Chip.Label>
                              {schedule.enabled ? "已启用" : "已停用"}
                            </Chip.Label>
                          </Chip>
                        </Table.Cell>
                        <Table.Cell>
                          {formatDate(schedule.next_run_at, schedule.timezone)}
                        </Table.Cell>
                        <Table.Cell>
                          <div className="flex items-center gap-1">
                            <TaskButtonLink
                              isIconOnly
                              aria-label="编辑计划"
                              href={`/tasks/schedules/${encodeURIComponent(schedule.id)}/edit${from}`}
                            >
                              <Pencil aria-hidden="true" />
                            </TaskButtonLink>
                            <Button
                              variant="ghost"
                              size="sm"
                              isIconOnly
                              aria-label={enabledLabel}
                              isDisabled={
                                mutation.isPending ||
                                (!schedule.enabled && !available)
                              }
                              onPress={() =>
                                mutation.mutate({ schedule, action: "toggle" })
                              }
                            >
                              {schedule.enabled ? (
                                <CirclePause aria-hidden="true" />
                              ) : (
                                <CirclePlay aria-hidden="true" />
                              )}
                            </Button>
                            <Button
                              variant="danger-soft"
                              size="sm"
                              isIconOnly
                              aria-label="删除计划"
                              isDisabled={mutation.isPending}
                              onPress={() => {
                                mutation.reset();
                                setDeleting(schedule);
                              }}
                            >
                              <Trash2 aria-hidden="true" />
                            </Button>
                          </div>
                        </Table.Cell>
                      </Table.Row>
                    );
                  }}
                </Table.Body>
              </Table.Content>
            </Table.ScrollContainer>
          </Table>
        )}
        <TaskPagination
          cursor={cursor}
          next={schedules.data?.next_cursor}
          onChange={(value) => change("cursor", value, false)}
        />
      </Panel>
      {deleting && (
        <ConfirmAction
          open
          onOpenChange={(open) => {
            if (!open) {
              setDeleting(null);
              mutation.reset();
            }
          }}
          title="删除定时计划"
          body={`确认删除「${deleting.name}」？已提交的任务可在任务中心查看。`}
          label="删除计划"
          pending={mutation.isPending}
          error={mutation.error}
          onConfirm={() =>
            mutation.mutate(
              { schedule: deleting, action: "delete" },
              { onSuccess: () => setDeleting(null) },
            )
          }
        />
      )}
    </main>
  );
}
