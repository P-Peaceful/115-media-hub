/** Fixed presentation fixtures, never a substitute for configuration or system responses. */
export const libraryDetailDemo = {
  statistics: { total: 12860, generated: 12532, pending: 298, failed: 30 },
  latestSync: {
    at: "2025-01-26T10:24:00+08:00",
    type: "增量同步",
    duration: "2 分 17 秒",
  },
  records: [
    {
      id: "incremental-26",
      at: "2025-01-26T10:24:00+08:00",
      type: "增量同步",
      summary: "完成同步，生成 128 个 STRM，更新 3 个，耗时 2 分 17 秒",
      result: "成功",
    },
    {
      id: "full-25",
      at: "2025-01-25T22:30:00+08:00",
      type: "全量同步",
      summary: "完成全量同步，生成 12,860 个 STRM，耗时 28 分 41 秒",
      result: "成功",
    },
    {
      id: "incremental-24",
      at: "2025-01-24T09:12:00+08:00",
      type: "增量同步",
      summary: "完成同步，生成 56 个 STRM，更新 1 个，耗时 1 分 03 秒",
      result: "成功",
    },
    {
      id: "preview-23",
      at: "2025-01-23T21:40:00+08:00",
      type: "执行预览",
      summary: "扫描目录，发现 132 个视频文件，预计生成 132 个 STRM",
      result: "完成",
    },
  ],
} as const;
