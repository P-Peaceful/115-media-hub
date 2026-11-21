import type { Chip } from "@heroui/react";
import {
  ArchiveRestore,
  CalendarClock,
  CircleCheck,
  CirclePause,
  CirclePlay,
  CircleX,
  Clock3,
  CloudUpload,
  Files,
  Link2,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  Trash2,
} from "lucide-react";

import type { components } from "@/lib/api/generated/schema";

type Schemas = components["schemas"];
export type TaskType = Schemas["TaskType"];
export type TaskStatus = Schemas["TaskStatus"];
export type TaskOutcome = Schemas["TaskOutcome"];
export type TaskAction = Schemas["TaskAction"];
export type ScheduleInput = Schemas["ScheduleInput"];

export const taskTypes = {
  scan: { label: "扫描", icon: Search, color: "success" },
  strm: { label: "STRM 生成", icon: Link2, color: "accent" },
  sync: { label: "同步", icon: RefreshCw, color: "default" },
  check: { label: "检查", icon: ShieldCheck, color: "default" },
  cleanup: { label: "清理", icon: Trash2, color: "default" },
  scrape: { label: "刮削", icon: Sparkles, color: "accent" },
  upload: { label: "回传", icon: CloudUpload, color: "default" },
} as const satisfies Record<
  TaskType,
  {
    label: string;
    icon: typeof Search;
    color: NonNullable<Chip["Props"]["color"]>;
  }
>;

export const taskStatuses = {
  running: { label: "运行中", icon: CirclePlay, color: "success" },
  queued: { label: "排队中", icon: Clock3, color: "default" },
  paused: { label: "已暂停", icon: CirclePause, color: "accent" },
  completed: { label: "已完成", icon: CircleCheck, color: "success" },
  failed: { label: "失败", icon: CircleX, color: "danger" },
  canceled: { label: "已取消", icon: CircleX, color: "default" },
} as const satisfies Record<
  TaskStatus,
  {
    label: string;
    icon: typeof Search;
    color: NonNullable<Chip["Props"]["color"]>;
  }
>;

export const taskActions = {
  pause: { label: "暂停任务", icon: CirclePause },
  resume: { label: "恢复任务", icon: CirclePlay },
  cancel: { label: "取消任务", icon: CircleX },
  retry: { label: "重试整个任务", icon: ArchiveRestore },
  retry_failed: { label: "仅重试失败项", icon: RefreshCw },
} as const satisfies Record<TaskAction, { label: string; icon: typeof Search }>;

export const outcomes = {
  success: "任务成功",
  failure: "任务失败",
  partial_failure: "部分失败",
} as const satisfies Record<TaskOutcome, string>;

export const taskSources = {
  manual: { label: "手动提交", icon: CirclePlay },
  schedule: { label: "定时计划", icon: CalendarClock },
  retry: { label: "失败重试", icon: Files },
} as const;

export const periods = {
  daily: "每天",
  weekly: "每周",
  monthly: "每月",
  custom: "自定义",
} as const satisfies Record<ScheduleInput["period"], string>;

export function hasKey<T extends object>(
  object: T,
  key: unknown,
): key is Extract<keyof T, string> {
  return typeof key === "string" && Object.hasOwn(object, key);
}

export function keys<T extends object>(object: T) {
  return Object.keys(object).filter((key) => hasKey(object, key));
}
