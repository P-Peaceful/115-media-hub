import { expect, test } from "@playwright/test";

const sampleLogs = [
  {
    id: "108",
    occurred_at: "2026-11-06T02:42:58Z",
    level: "info",
    module: "sync",
    operation: "incremental_sync",
    job_id: "8721",
    request_id: "req-8721",
    error_code: null,
    message: "开始执行增量同步，媒体库：电影库",
    details: { library_id: "lib-movie" },
  },
  {
    id: "107",
    occurred_at: "2026-11-06T02:42:36Z",
    level: "info",
    module: "scan",
    operation: "scan_directory",
    job_id: "8720",
    request_id: null,
    error_code: null,
    message: "扫描完成，共发现 128 个文件（新增 12，更新 3，跳过 113）",
    details: { discovered: 128 },
  },
  {
    id: "106",
    occurred_at: "2026-11-06T02:41:59Z",
    level: "warn",
    module: "strm",
    operation: "generate_strm",
    job_id: "8719",
    request_id: null,
    error_code: "strm_exists",
    message: "文件已存在，跳过生成：/Movies/星际穿越 (2014).strm",
    details: {},
  },
  {
    id: "105",
    occurred_at: "2026-11-06T02:40:11Z",
    level: "error",
    module: "upload",
    operation: "upload_sidecar",
    job_id: "8718",
    request_id: "req-err-1",
    error_code: "upload_timeout",
    message: "文件上传失败：网络超时（3 次重试后仍失败）：poster.jpg",
    details: { attempt: 3 },
  },
  {
    id: "104",
    occurred_at: "2026-11-06T02:38:05Z",
    level: "info",
    module: "system",
    operation: "check_auth",
    job_id: null,
    request_id: null,
    error_code: null,
    message: "115 授权检测通过，授权有效期至 2026-12-05 18:00:00",
    details: {},
  },
];

const sampleTasks = [
  {
    id: "8721",
    type: "sync",
    library_id: "lib-movie",
    library_name: "电影库",
    source: "schedule",
    schedule_id: null,
    parent_id: null,
    status: "running",
    outcome: null,
    created_at: "2026-11-06T02:42:00Z",
    started_at: "2026-11-06T02:42:58Z",
    finished_at: null,
    duration_ms: null,
    progress_percent: 45,
    speed_bytes_per_second: null,
    current_file: null,
    total_items: 128,
    succeeded_items: 50,
    failed_items: 0,
    skipped_items: 0,
    actions: ["cancel"],
  },
];

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/auth/bootstrap", (route) =>
    route.fulfill({
      json: {
        initialized: true,
        authenticated: true,
        administrator: { id: "1", username: "admin" },
        expires_at: "2099-01-01T00:00:00Z",
        csrf_token: "e2e-csrf-token",
      },
    }),
  );
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({
      json: {
        status: "valid",
        profile: {
          user_name: "测试账号",
          user_id: "13812345678",
          avatar: null,
          is_member: true,
          member_expires_at: null,
          member_forever: false,
        },
        space: null,
        space_status: "unavailable",
        devices: [],
        devices_status: "unavailable",
      },
    }),
  );
  await page.route("**/api/v1/events", (route) =>
    route.fulfill({ status: 204 }),
  );
  await page.route("**/api/v1/logs/stream**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: ": connected\n\n",
    }),
  );
  await page.route("**/api/v1/tasks?**", (route) =>
    route.fulfill({
      json: {
        items: sampleTasks,
        next_cursor: null,
        total: sampleTasks.length,
        by_type: { sync: 1 },
        by_status: { running: 1 },
      },
    }),
  );
});

test("侧栏进入运行日志页，支持多维筛选、高亮错误与详情展开", async ({
  page,
}) => {
  await page.route("**/api/v1/logs?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    const level = search.get("level");
    const module = search.get("module");
    const q = (search.get("q") ?? "").trim();
    const items = sampleLogs.filter((entry) => {
      if (level && entry.level !== level) return false;
      if (module && entry.module !== module) return false;
      if (q && !entry.message.includes(q)) return false;
      return true;
    });
    return route.fulfill({ json: { items, next_cursor: null } });
  });

  await page.goto("/logs");
  await expect(page.getByRole("heading", { name: "运行日志" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "面包屑" })).toContainText(
    "任务与维护",
  );
  await expect(
    page.getByText("开始执行增量同步，媒体库：电影库"),
  ).toBeVisible();
  await expect(
    page.getByText("文件上传失败：网络超时（3 次重试后仍失败）：poster.jpg"),
  ).toBeVisible();

  const errorRow = page.getByRole("row", { name: /文件上传失败/ });
  const errorBox = errorRow.locator('[data-level="error"]');
  await expect(errorBox).toHaveAttribute("data-highlighted", "true");
  await page.getByText("高亮错误", { exact: true }).click();
  await expect(errorBox).toHaveAttribute("data-highlighted", "false");
  await page.getByRole("checkbox", { name: "高亮错误" }).press("Space");
  await expect(errorBox).toHaveAttribute("data-highlighted", "true");

  await errorRow.getByRole("button", { name: "展开日志详情" }).click();
  await expect(page.getByText("upload_timeout")).toBeVisible();

  const searchInput = page.getByRole("searchbox", { name: "搜索关键词" });
  await searchInput.fill("星际穿越");
  await expect(
    page.getByText("文件已存在，跳过生成：/Movies/星际穿越 (2014).strm"),
  ).toBeVisible();
  await expect(page.getByText("开始执行增量同步，媒体库：电影库")).toHaveCount(
    0,
  );

  await expect(
    page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).resolves.toBe(true);
});

test("支持下载日志下拉菜单与清理日志范围弹窗", async ({ page }) => {
  let logs = [...sampleLogs];
  let exportFormat = "";
  let clearPayload: { scope?: string } | null = null;
  let clearCsrf = "";

  await page.route("**/api/v1/logs?**", (route) =>
    route.fulfill({ json: { items: logs, next_cursor: null } }),
  );
  await page.route("**/api/v1/logs/export?**", (route) => {
    exportFormat =
      new URL(route.request().url()).searchParams.get("format") ?? "";
    return route.fulfill({
      status: 200,
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        "Content-Disposition":
          'attachment; filename="mediahub-logs-20261106.log"',
        "X-Export-Truncated": "false",
      },
      body: "2026-11-06 10:42:58 [INFO] [同步] [#8721] 开始执行增量同步\n",
    });
  });
  await page.route("**/api/v1/logs/clear", async (route) => {
    clearCsrf = route.request().headers()["x-csrf-token"] ?? "";
    clearPayload = route.request().postDataJSON() as { scope?: string };
    logs = [];
    return route.fulfill({
      json: { deleted_count: 5, scope: clearPayload.scope ?? "before_7d" },
    });
  });

  await page.goto("/logs");
  await expect(
    page.getByText("开始执行增量同步，媒体库：电影库"),
  ).toBeVisible();

  await page.getByRole("button", { name: "下载日志" }).click();
  await page
    .getByRole("menuitem", { name: "下载当前筛选日志（纯文本 .log）" })
    .click();
  await expect(
    page.getByText("已开始下载 mediahub-logs-20261106.log。"),
  ).toBeVisible();
  expect(exportFormat).toBe("log");

  await page.getByRole("button", { name: "清理日志" }).click();
  const dialog = page.getByRole("alertdialog", { name: "清理运行日志" });
  await expect(dialog).toBeVisible();
  await dialog.getByText("清理 30 天前", { exact: true }).click();
  await expect(
    dialog.getByRole("radio", { name: /清理 30 天前/ }),
  ).toBeChecked();
  await dialog.getByRole("button", { name: "确认清理" }).click();

  await expect(page.getByText("已清理 5 条运行日志。")).toBeVisible();
  expect(clearCsrf).toBe("e2e-csrf-token");
  expect(clearPayload).toEqual({ scope: "before_30d" });
  await expect(page.getByText("当前时间范围内暂无运行日志")).toBeVisible();
});
