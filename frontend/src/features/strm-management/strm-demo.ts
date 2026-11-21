export const strmLibraries = { movie: "电影", tv: "电视剧" } as const;
export const strmStatusLabels = {
  valid: "有效",
  invalid: "异常",
  pending: "待处理",
} as const;
export const strmSourceStatusLabels = {
  linked: "已关联",
  unlinked: "未关联",
} as const;
export const strmPlaybackStatusLabels = {
  playable: "可播放",
  unplayable: "不可播放",
  unchecked: "未检查",
} as const;

export type StrmLibrary = keyof typeof strmLibraries;
export type StrmStatus = keyof typeof strmStatusLabels;
export type StrmSourceStatus = keyof typeof strmSourceStatusLabels;
export type StrmPlaybackStatus = keyof typeof strmPlaybackStatusLabels;
export type StrmCleanupDisposition = "keep" | "remove" | "regenerate";

export type StrmFile = {
  id: string;
  name: string;
  relativePath: string;
  title: string;
  localPath: string;
  sourcePath: string;
  library: StrmLibrary;
  status: StrmStatus;
  sourceStatus: StrmSourceStatus;
  playbackStatus: StrmPlaybackStatus;
  generatedAt: string;
  lastCheckedAt: string | null;
  sizeBytes: number;
  content: string;
  reason: string | null;
  cleanupDisposition: StrmCleanupDisposition;
};

const sampleStates = {
  healthy: {
    status: "valid",
    sourceStatus: "linked",
    playbackStatus: "playable",
    reason: null,
    cleanupDisposition: "keep",
  },
  missingSource: {
    status: "invalid",
    sourceStatus: "unlinked",
    playbackStatus: "unplayable",
    reason: "源文件不存在或已被移动",
    cleanupDisposition: "remove",
  },
  brokenLink: {
    status: "invalid",
    sourceStatus: "linked",
    playbackStatus: "unplayable",
    reason: "播放链接返回 404",
    cleanupDisposition: "regenerate",
  },
  unchecked: {
    status: "pending",
    sourceStatus: "linked",
    playbackStatus: "unchecked",
    reason: "未完成播放链接检查",
    cleanupDisposition: "regenerate",
  },
} as const satisfies Record<
  string,
  Pick<
    StrmFile,
    | "status"
    | "sourceStatus"
    | "playbackStatus"
    | "reason"
    | "cleanupDisposition"
  >
>;

type SampleState = keyof typeof sampleStates;
type SampleFile = {
  id: string;
  title: string;
  generatedAt: string;
  state: SampleState;
  episode?: string;
};

function createSampleFile({
  id,
  title,
  generatedAt,
  state,
  episode,
}: SampleFile): StrmFile {
  const library = episode ? "tv" : "movie";
  const stem = episode ? `${title}/${episode}` : title;
  const path = `${strmLibraries[library]}/${stem}`;
  const displayTitle = episode ? `${title} ${episode}` : title;
  const content = [
    "#EXTM3U",
    `#EXTINF:-1,${displayTitle}`,
    `https://media.example.com/p/${id}`,
    "",
  ].join("\n");

  return {
    id,
    title,
    library,
    name: `${episode ?? title}.strm`,
    relativePath: `${path}.strm`,
    localPath: `/115/Media/${path}.strm`,
    sourcePath: `/115/Video/${path}.mkv`,
    generatedAt,
    lastCheckedAt:
      state === "unchecked"
        ? null
        : new Date(Date.parse(generatedAt) + 3 * 60_000).toISOString(),
    sizeBytes: new TextEncoder().encode(content).byteLength,
    content,
    ...sampleStates[state],
  };
}

const featuredFiles = [
  {
    id: "wandering-earth-2",
    title: "流浪地球2 (2023)",
    state: "healthy",
    minutesAgo: 15,
  },
  {
    id: "oppenheimer",
    title: "奥本海默 (2023)",
    state: "healthy",
    minutesAgo: 18,
  },
  {
    id: "three-body-s01e01",
    title: "三体 (2023)",
    episode: "S01E01",
    state: "unchecked",
    daysAgo: 2,
    hour: 22,
    minute: 43,
  },
  {
    id: "dune-2",
    title: "沙丘2 (2024)",
    state: "missingSource",
    daysAgo: 2,
    hour: 20,
    minute: 11,
  },
  {
    id: "blossoms-s01e01",
    title: "繁花 (2023)",
    episode: "S01E01",
    state: "healthy",
    daysAgo: 3,
    hour: 19,
    minute: 2,
  },
  {
    id: "three-body-s01e02",
    title: "三体 (2023)",
    episode: "S01E02",
    state: "brokenLink",
    daysAgo: 3,
    hour: 18,
    minute: 45,
  },
  {
    id: "blossoms-s01e03",
    title: "繁花 (2023)",
    episode: "S01E03",
    state: "unchecked",
    daysAgo: 3,
    hour: 18,
    minute: 36,
  },
] as const;

const additionalMovies = [
  { id: "arrival", title: "降临 (2016)", state: "healthy" },
  { id: "interstellar", title: "星际穿越 (2014)", state: "healthy" },
  { id: "hero", title: "英雄 (2002)", state: "missingSource" },
  { id: "spirited-away", title: "千与千寻 (2001)", state: "healthy" },
  { id: "inception", title: "盗梦空间 (2010)", state: "brokenLink" },
  { id: "mars", title: "火星救援 (2015)", state: "healthy" },
  { id: "suzume", title: "铃芽之旅 (2022)", state: "unchecked" },
] as const;

const series = [
  { id: "long-season", title: "漫长的季节 (2023)" },
  { id: "bad-kids", title: "隐秘的角落 (2020)" },
  { id: "the-knockout", title: "狂飙 (2023)" },
  { id: "reset", title: "开端 (2022)" },
] as const;

const episodeStates: SampleState[] = [
  "healthy",
  "healthy",
  "healthy",
  "healthy",
  "healthy",
  "healthy",
  "healthy",
  "healthy",
  "missingSource",
  "brokenLink",
  "unchecked",
  "healthy",
];

export function createStrmDemoFiles(today = new Date()): StrmFile[] {
  function sampleTime(daysAgo: number, hour: number, minute: number) {
    const date = new Date(today);
    date.setDate(date.getDate() - daysAgo);
    date.setHours(hour, minute, 34, 0);
    return date.toISOString();
  }

  return [
    ...featuredFiles.map((file) => {
      const generatedAt =
        "minutesAgo" in file
          ? new Date(today.getTime() - file.minutesAgo * 60_000).toISOString()
          : sampleTime(file.daysAgo, file.hour, file.minute);
      return createSampleFile({ ...file, generatedAt });
    }),
    ...additionalMovies.map((file, index) =>
      createSampleFile({ ...file, generatedAt: sampleTime(4 + index, 16, 0) }),
    ),
    ...series.flatMap(({ id, title }, seriesIndex) =>
      episodeStates.map((state, episodeIndex) => {
        const episode = `S01E${String(episodeIndex + 1).padStart(2, "0")}`;
        return createSampleFile({
          id: `${id}-${episode.toLowerCase()}`,
          title,
          episode,
          state,
          generatedAt: sampleTime(7 + seriesIndex * 9 + episodeIndex, 12, 0),
        });
      }),
    ),
  ];
}

export function summarizeStrmFiles(files: readonly StrmFile[]) {
  const totals = { total: files.length, valid: 0, invalid: 0, pending: 0 };
  for (const file of files) totals[file.status]++;
  return totals;
}

export function canRegenerateStrmFile(file: StrmFile) {
  return file.cleanupDisposition === "regenerate" && file.status !== "valid";
}

export function regenerateStrmFile(
  file: StrmFile,
  generatedAt: string,
): StrmFile {
  return {
    ...file,
    ...sampleStates.healthy,
    generatedAt,
    lastCheckedAt: generatedAt,
  };
}

export type StrmCleanupPreview = {
  libraries: StrmLibrary[];
  total: number;
  releaseBytes: number;
  removableIds: string[];
  rows: { kind: StrmCleanupDisposition; count: number; bytes: number }[];
};

export function previewStrmCleanup(
  files: readonly StrmFile[],
  ids: Iterable<string>,
): StrmCleanupPreview {
  const selectedIds = new Set(ids);
  const libraries = new Set<StrmLibrary>();
  const removableIds: string[] = [];
  const rows = {
    keep: { kind: "keep", count: 0, bytes: 0 },
    remove: { kind: "remove", count: 0, bytes: 0 },
    regenerate: { kind: "regenerate", count: 0, bytes: 0 },
  } satisfies Record<
    StrmCleanupDisposition,
    StrmCleanupPreview["rows"][number]
  >;
  let total = 0;

  for (const file of files) {
    if (!selectedIds.has(file.id)) continue;

    // Removal requires both an abnormal result and explicit cleanup eligibility.
    const removable =
      file.status === "invalid" && file.cleanupDisposition === "remove";
    let kind: StrmCleanupDisposition = "keep";
    if (removable) {
      removableIds.push(file.id);
      kind = "remove";
    } else if (canRegenerateStrmFile(file)) {
      kind = "regenerate";
    }
    const row = rows[kind];
    row.count++;
    row.bytes += file.sizeBytes;
    total++;
    libraries.add(file.library);
  }

  return {
    libraries: [...libraries],
    total,
    releaseBytes: rows.remove.bytes,
    removableIds,
    rows: Object.values(rows),
  };
}

export function formatStrmDate(value: string | null, withSeconds = false) {
  if (value === null) return "—";

  const date = new Date(value);
  const pad = (part: number) => String(part).padStart(2, "0");
  const day = [date.getFullYear(), date.getMonth() + 1, date.getDate()]
    .map(pad)
    .join("-");
  const time = [date.getHours(), date.getMinutes()];
  if (withSeconds) time.push(date.getSeconds());
  return `${day} ${time.map(pad).join(":")}`;
}

export function formatStrmSize(bytes: number) {
  if (bytes >= 2 ** 20) return `${(bytes / 2 ** 20).toFixed(1)} MB`;
  if (bytes >= 2 ** 10) return `${(bytes / 2 ** 10).toFixed(1)} KB`;
  return `${bytes} B`;
}

export function downloadStrmCsv(files: readonly StrmFile[]) {
  const rows = [
    ["文件名称", "所属媒体库", "状态", "生成时间", "源文件关联", "播放链接"],
    ...files.map((file) => [
      file.relativePath,
      strmLibraries[file.library],
      strmStatusLabels[file.status],
      formatStrmDate(file.generatedAt, true),
      strmSourceStatusLabels[file.sourceStatus],
      strmPlaybackStatusLabels[file.playbackStatus],
    ]),
  ];
  const csv = rows
    .map((row) =>
      row.map((value) => `"${value.replaceAll('"', '""')}"`).join(","),
    )
    .join("\r\n");
  const url = URL.createObjectURL(
    new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = "strm-files.csv";
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}
