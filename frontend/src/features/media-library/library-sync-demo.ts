/** Presentation fixtures for the media library operation pages. */
export const changeKinds = ["added", "updated", "skipped", "deleted"] as const;
export type ChangeKind = (typeof changeKinds)[number];
type ChangeCounts = Record<ChangeKind, number>;

export const changeLabels: Record<ChangeKind, string> = {
  added: "新增",
  updated: "更新",
  skipped: "跳过",
  deleted: "删除",
};

export const estimatedChanges: ChangeCounts = {
  added: 1248,
  updated: 128,
  skipped: 28560,
  deleted: 62,
};

export const estimatedTotal = changeKinds.reduce(
  (total, kind) => total + estimatedChanges[kind],
  0,
);

const changeDetails: Record<ChangeKind, string> = {
  added: "新增媒体文件",
  updated: "更新 NFO 与海报",
  skipped: "无变更，跳过",
  deleted: "源文件不存在，将移除",
};

const previewSamples: [ChangeKind, string][] = [
  ["added", "沙丘 2 (2024)"],
  ["updated", "奥本海默 (2023)"],
  ["skipped", "流浪地球 2 (2023)"],
  ["added", "爱乐之城 (2016)"],
  ["deleted", "旧版样片 (2018)"],
  ["skipped", "星际穿越 (2014)"],
  ["added", "盗梦空间 (2010)"],
  ["updated", "降临 (2016)"],
  ["skipped", "银翼杀手 2049 (2017)"],
  ["added", "火星救援 (2015)"],
  ["skipped", "头号玩家 (2018)"],
  ["skipped", "地心引力 (2013)"],
  ["deleted", "历史测试片 (2019)"],
  ["updated", "普罗米修斯 (2012)"],
  ["skipped", "千与千寻 (2001)"],
  ["added", "海街日记 (2015)"],
  ["skipped", "你的名字。 (2016)"],
  ["added", "红辣椒 (2006)"],
  ["skipped", "天空之城 (1986)"],
  ["skipped", "龙猫 (1988)"],
  ["deleted", "旧版演示片 (2020)"],
  ["updated", "借东西的小人阿莉埃蒂 (2010)"],
  ["added", "哈尔的移动城堡 (2004)"],
];

export const previewChanges = previewSamples.map(([kind, title]) => ({
  kind,
  title,
  entry: title + "/",
  detail: changeDetails[kind],
}));

type SyncRecord = {
  at: string;
  duration: string;
  result: "成功" | "失败";
  counts: ChangeCounts;
};

export const fullSyncHistory: readonly [SyncRecord, ...SyncRecord[]] = [
  {
    at: "2025-01-25T21:36:00+08:00",
    duration: "12 分 47 秒",
    result: "成功",
    counts: estimatedChanges,
  },
  {
    at: "2025-01-10T03:12:00+08:00",
    duration: "14 分 22 秒",
    result: "成功",
    counts: { added: 1103, updated: 97, skipped: 28712, deleted: 51 },
  },
  {
    at: "2024-12-26T20:08:00+08:00",
    duration: "13 分 19 秒",
    result: "成功",
    counts: { added: 986, updated: 112, skipped: 28901, deleted: 47 },
  },
  {
    at: "2024-12-12T10:40:00+08:00",
    duration: "4 分 03 秒",
    result: "失败",
    counts: { added: 233, updated: 14, skipped: 6245, deleted: 0 },
  },
  {
    at: "2024-11-24T20:18:00+08:00",
    duration: "11 分 48 秒",
    result: "成功",
    counts: { added: 578, updated: 71, skipped: 28216, deleted: 31 },
  },
];

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
