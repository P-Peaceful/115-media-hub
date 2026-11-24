import { requestJson } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/schema";

import type { ScheduleInput, TaskAction, TaskType } from "./task-kinds";
import {
  hasKey,
  outcomes,
  periods,
  taskActions,
  taskSources,
  taskStatuses,
  taskTypes,
} from "./task-kinds";

type Schemas = components["schemas"];
export type Task = Schemas["Task"];
export type TaskTypeCapability = Schemas["TaskTypeCapability"];
export type Schedule = Schemas["Schedule"];

const invalidResponse = () =>
  new ApiError("invalid_response", "服务返回了无效响应，请稍后重试。");

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
const isString = (value: unknown) => typeof value === "string";
const isBoolean = (value: unknown) => typeof value === "boolean";
function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function isMetric(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}
const isTime = (value: unknown) =>
  typeof value === "string" && Number.isFinite(Date.parse(value));

function nullable(value: unknown, check: (value: unknown) => boolean) {
  return value === null || check(value);
}
function optional(value: unknown, check: (value: unknown) => boolean) {
  return value === undefined || check(value);
}
function isArrayOf<T>(
  value: unknown,
  check: (item: unknown) => item is T,
): value is T[] {
  return Array.isArray(value) && value.every(check);
}

function isTask(value: unknown): value is Task {
  return (
    isRecord(value) &&
    isString(value.id) &&
    hasKey(taskTypes, value.type) &&
    isString(value.library_id) &&
    isString(value.library_name) &&
    hasKey(taskSources, value.source) &&
    hasKey(taskStatuses, value.status) &&
    nullable(value.schedule_id, isString) &&
    nullable(value.parent_id, isString) &&
    nullable(value.outcome, (item) => hasKey(outcomes, item)) &&
    isTime(value.created_at) &&
    nullable(value.started_at, isTime) &&
    nullable(value.finished_at, isTime) &&
    nullable(value.duration_ms, isCount) &&
    nullable(value.progress_percent, (item) => isMetric(item) && item <= 100) &&
    nullable(value.speed_bytes_per_second, isMetric) &&
    nullable(value.current_file, isString) &&
    nullable(value.total_items, isCount) &&
    isCount(value.succeeded_items) &&
    isCount(value.failed_items) &&
    isCount(value.skipped_items) &&
    isArrayOf(value.actions, (item) => hasKey(taskActions, item))
  );
}

function isCapability(value: unknown): value is TaskTypeCapability {
  return (
    isRecord(value) &&
    hasKey(taskTypes, value.type) &&
    isBoolean(value.available) &&
    nullable(value.reason, isString) &&
    isBoolean(value.can_pause) &&
    isBoolean(value.can_retry_failed) &&
    isBoolean(value.can_schedule)
  );
}

function isSchedule(value: unknown): value is Schedule {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.name) &&
    isCount(value.revision) &&
    value.revision >= 1 &&
    hasKey(taskTypes, value.type) &&
    nullable(value.library_id, isString) &&
    isBoolean(value.enabled) &&
    hasKey(periods, value.period) &&
    isString(value.timezone) &&
    optional(value.time, isString) &&
    optional(value.weekdays, (items) =>
      isArrayOf(items, (item): item is number => isCount(item) && item <= 6),
    ) &&
    optional(
      value.month_day,
      (item) => isCount(item) && item >= 1 && item <= 31,
    ) &&
    optional(value.cron, isString) &&
    isArrayOf(value.notify_on, (item) => hasKey(outcomes, item)) &&
    nullable(value.next_run_at, isTime) &&
    isTime(value.created_at) &&
    isTime(value.updated_at)
  );
}
function isFailure(value: unknown): value is Schemas["TaskFailure"] {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.path) &&
    isString(value.reason)
  );
}
function isLog(value: unknown): value is Schemas["TaskLog"] {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.level) &&
    isString(value.message) &&
    isTime(value.created_at)
  );
}
function isCounts(value: unknown): value is Record<string, number> {
  return isRecord(value) && Object.values(value).every(isCount);
}

function decode<T>(value: unknown, check: (value: unknown) => value is T): T {
  if (!check(value)) throw invalidResponse();
  return value;
}

function decodePage<T>(value: unknown, check: (item: unknown) => item is T) {
  if (
    !isRecord(value) ||
    !isArrayOf(value.items, check) ||
    !(value.next_cursor === null || typeof value.next_cursor === "string")
  ) {
    throw invalidResponse();
  }
  return { items: value.items, next_cursor: value.next_cursor };
}

const tasksURL = "/api/v1/tasks";
const schedulesURL = "/api/v1/schedules";
const rowURL = (base: string, id: string) =>
  `${base}/${encodeURIComponent(id)}`;

function listURL(base: string, params: URLSearchParams) {
  const search = new URLSearchParams(params);
  search.set("limit", "25");
  return `${base}?${search}`;
}

export async function listCapabilities(signal: AbortSignal) {
  const value = await requestJson("/api/v1/task-types", { signal });
  if (!isRecord(value) || !isArrayOf(value.items, isCapability))
    throw invalidResponse();
  return value.items;
}

export async function listTasks(params: URLSearchParams, signal: AbortSignal) {
  const value = await requestJson(listURL(tasksURL, params), { signal });
  const page = decodePage(value, isTask);
  if (
    !isRecord(value) ||
    !isCount(value.total) ||
    !isCounts(value.by_type) ||
    !isCounts(value.by_status)
  )
    throw invalidResponse();
  return {
    ...page,
    total: value.total,
    by_type: value.by_type,
    by_status: value.by_status,
  };
}

export async function getTask(id: string, signal: AbortSignal) {
  return decode(await requestJson(rowURL(tasksURL, id), { signal }), isTask);
}

export async function listFailures(
  id: string,
  params: URLSearchParams,
  signal: AbortSignal,
) {
  return decodePage(
    await requestJson(listURL(`${rowURL(tasksURL, id)}/failures`, params), {
      signal,
    }),
    isFailure,
  );
}
export async function listLogs(
  id: string,
  params: URLSearchParams,
  signal: AbortSignal,
) {
  return decodePage(
    await requestJson(listURL(`${rowURL(tasksURL, id)}/logs`, params), {
      signal,
    }),
    isLog,
  );
}

function mutate(
  path: string,
  method: string,
  csrf: string,
  payload?: object,
  revision?: number,
) {
  return requestJson(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
      ...(revision !== undefined
        ? { "X-Schedule-Revision": String(revision) }
        : {}),
    },
    ...(payload ? { body: JSON.stringify(payload) } : {}),
  });
}

export async function submitTask(
  csrf: string,
  type: TaskType,
  libraryId: string,
) {
  return decode(
    await mutate(tasksURL, "POST", csrf, { type, library_id: libraryId }),
    isTask,
  );
}

export async function actOnTask(csrf: string, id: string, action: TaskAction) {
  return decode(
    await mutate(`${rowURL(tasksURL, id)}/actions`, "POST", csrf, { action }),
    isTask,
  );
}

export async function listSchedules(
  params: URLSearchParams,
  signal: AbortSignal,
) {
  return decodePage(
    await requestJson(listURL(schedulesURL, params), { signal }),
    isSchedule,
  );
}
export async function getSchedule(id: string, signal: AbortSignal) {
  return decode(
    await requestJson(rowURL(schedulesURL, id), { signal }),
    isSchedule,
  );
}
export async function saveSchedule(
  csrf: string,
  current: { id: string; revision: number } | null,
  input: ScheduleInput,
) {
  return decode(
    await mutate(
      current ? rowURL(schedulesURL, current.id) : schedulesURL,
      current ? "PUT" : "POST",
      csrf,
      input,
      current?.revision,
    ),
    isSchedule,
  );
}
export async function setScheduleEnabled(
  csrf: string,
  id: string,
  enabled: boolean,
) {
  return decode(
    await mutate(`${rowURL(schedulesURL, id)}/enabled`, "PATCH", csrf, {
      enabled,
    }),
    isSchedule,
  );
}
export async function deleteSchedule(csrf: string, id: string) {
  await mutate(rowURL(schedulesURL, id), "DELETE", csrf);
}
