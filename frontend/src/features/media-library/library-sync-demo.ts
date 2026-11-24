/** Presentation fixtures for the media library operation pages. */
export type ChangeKind = "added" | "updated" | "skipped" | "deleted";

export const changeLabels: Record<ChangeKind, string> = {
  added: "新增",
  updated: "更新",
  skipped: "跳过",
  deleted: "删除",
};

export const incrementalOverview = {
  total: 342,
  pending: 3,
  succeeded: 338,
  failed: 1,
};

type TriggerRecord = {
  time: string;
  kind: Exclude<ChangeKind, "skipped">;
  entry: string;
  result: "成功" | "失败";
};

export const triggerHistory: TriggerRecord[] = [
  {
    time: "10:24:36",
    kind: "added",
    entry: "沙丘 2 (2024).mkv",
    result: "成功",
  },
  {
    time: "10:21:09",
    kind: "updated",
    entry: "奥本海默 (2023).nfo",
    result: "成功",
  },
  {
    time: "10:18:47",
    kind: "deleted",
    entry: "旧版样片 (2018)/",
    result: "成功",
  },
  {
    time: "10:17:02",
    kind: "added",
    entry: "测试短片 (2024).mkv",
    result: "失败",
  },
  {
    time: "10:05:12",
    kind: "added",
    entry: "爱乐之城 (2016).mkv",
    result: "成功",
  },
  {
    time: "09:57:03",
    kind: "updated",
    entry: "流浪地球 2 (2023).jpg",
    result: "成功",
  },
  {
    time: "09:53:27",
    kind: "updated",
    entry: "星际穿越 (2014).nfo",
    result: "成功",
  },
  { time: "09:49:10", kind: "added", entry: "降临 (2016).mkv", result: "成功" },
  {
    time: "09:45:29",
    kind: "updated",
    entry: "盗梦空间 (2010).jpg",
    result: "成功",
  },
  {
    time: "09:43:12",
    kind: "added",
    entry: "火星救援 (2015).mkv",
    result: "成功",
  },
  {
    time: "09:42:22",
    kind: "deleted",
    entry: "历史测试片 (2019)/",
    result: "成功",
  },
  {
    time: "09:40:15",
    kind: "added",
    entry: "海街日记 (2015).mkv",
    result: "成功",
  },
];

export const incrementalAlerts = [
  {
    title: "事件处理失败",
    time: "10:17:02",
    severity: "danger",
    summary: "STRM 生成失败（ID 848291），写入输出文件时出错。",
    instruction: "检查输出目录是否存在，以及应用进程是否有写入权限。",
  },
  {
    title: "轮询延迟偏高",
    time: "09:44:10",
    severity: "warning",
    summary: "本次轮询耗时 8.6 秒，超过演示中的预期耗时。",
    instruction: "可以在上方试用更长的轮询间隔或更低的批量处理上限。",
  },
  {
    title: "目录变更待处理",
    time: "09:31:26",
    severity: "warning",
    summary: "示例目录已重命名，下次轮询时将重新检查目录。",
    instruction: "确认当前媒体库绑定的目录，并检查是否开启了包含目录变更。",
  },
] as const;

export function formatSyncCount(count: number) {
  return count.toLocaleString("en-US");
}

export function sampleMediaPath(directory: string, entry: string) {
  return directory.replace(/\/+$/, "") + "/" + entry;
}
