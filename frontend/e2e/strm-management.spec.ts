import { readFile } from "node:fs/promises";

import { expect, test } from "@playwright/test";

import type { components } from "../src/lib/api/generated/schema";

type Schemas = components["schemas"];
const now = "2026-11-23T00:00:00Z";
const library: Schemas["Library"] = {
  id: "movie-library",
  revision: 1,
  created_at: now,
  updated_at: now,
  name: "电影库",
  directory_id: "123",
  directory_path: "/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/STRM/电影",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_nfo: false,
  generate_poster_cover: false,
  language_preference: "zh-CN",
  scrape_source: "none",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: false,
  file_encoding: "UTF-8",
  link_type: "proxy",
  strm_direct_domain: "http://mediahub.example:8080",
  strm_link_format: "original_name",
};
const first: Schemas["StrmFile"] = {
  id: "strm-1",
  library_id: library.id,
  library_name: library.name,
  name: "流浪地球2.strm",
  relative_path: "科幻/流浪地球2.strm",
  source_file_id: "9007199254740993",
  source_path: "/电影/科幻/流浪地球2.mkv",
  generated_at: now,
  size_bytes: 98,
};
const data: Schemas["StrmFile"][] = Array.from({ length: 26 }, (_, index) =>
  index === 0
    ? first
    : {
        ...first,
        id: `strm-${index + 1}`,
        name: `片名-${index}.strm`,
        relative_path: `片名-${index}.strm`,
      },
);
const completed: Schemas["Task"] = {
  id: "generated-task",
  type: "strm",
  library_id: library.id,
  library_name: library.name,
  source: "manual",
  schedule_id: null,
  parent_id: null,
  status: "completed",
  outcome: "success",
  created_at: now,
  started_at: now,
  finished_at: now,
  duration_ms: 500,
  progress_percent: 100,
  speed_bytes_per_second: null,
  current_file: null,
  total_items: 26,
  succeeded_items: 26,
  failed_items: 0,
  skipped_items: 0,
  actions: [],
};
const capability: Schemas["TaskTypeCapability"] = {
  type: "strm",
  available: true,
  reason: null,
  can_pause: false,
  can_retry_failed: false,
  can_schedule: false,
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/auth/bootstrap", (route) =>
    route.fulfill({
      json: {
        initialized: true,
        authenticated: true,
        administrator: { id: "1", username: "admin" },
        expires_at: "2099-01-01T00:00:00Z",
        csrf_token: "token",
      },
    }),
  );
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({
      json: {
        status: "unconfigured",
        profile: null,
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
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({ json: { items: [library], next_cursor: null } }),
  );
  await page.route("**/api/v1/strm?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    const q = search.get("q") ?? "";
    const libraryId = search.get("library_id") ?? library.id;
    const filtered =
      libraryId === library.id
        ? data.filter((file) => file.relative_path.includes(q))
        : [];
    const page = Number(search.get("page") ?? 1);
    const limit = Number(search.get("limit") ?? 50);
    return route.fulfill({
      json: {
        items: filtered.slice((page - 1) * limit, page * limit),
        total: filtered.length,
        page,
        limit,
      } satisfies Schemas["StrmPage"],
    });
  });
});

test("真实清单支持分页和按库搜索，详情返回时恢复筛选", async ({
  page,
}, testInfo) => {
  const content = "http://mediahub.example/p/strm-1/playback-token/movie.mkv\n";
  await page.route("**/api/v1/strm/strm-1", (route) =>
    route.fulfill({
      json: {
        ...first,
        output_path: "/STRM/电影/科幻/流浪地球2.strm",
        file_state: "present",
        content,
      } satisfies Schemas["StrmDetail"],
    }),
  );
  const main = page.getByRole("main");
  await page.goto("/strm?library_id=movie-library&size=25");
  await expect(main.getByRole("heading", { name: "STRM 管理" })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme",
    testInfo.project.name === "mobile-dark" ? "dark" : "light",
  );
  await expect(main.getByText("共 26 条")).toBeVisible();
  const file = main.getByRole("row", { name: /流浪地球2/ });
  await expect(file).toContainText("未检查");
  await main.getByRole("button", { name: "下一页" }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(file).toHaveCount(0);
  await expect(main.getByRole("row", { name: /片名-25/ })).toBeVisible();
  const search = main.getByRole("searchbox", { name: "搜索 STRM 文件" });
  await search.fill("流浪地球");
  await expect(main.getByText("共 1 条")).toBeVisible();
  await expect(main.getByRole("button", { name: "下一页" })).toBeDisabled();
  const fileName = file.getByRole("link");
  await fileName.focus();
  await page.keyboard.press("Enter");
  await expect(main.getByRole("heading", { name: "文件详情" })).toBeVisible();
  await expect(main.locator("pre")).toContainText(content.trim());
  await expect(main.getByText(first.source_file_id)).toBeVisible();
  await expect(main.getByText("演示数据", { exact: true })).toHaveCount(0);
  await main.getByRole("link", { name: "返回 STRM 管理" }).click();
  await expect(search).toHaveValue("流浪地球");
  await expect(page).toHaveURL(/library_id=movie-library&size=25&q=/);
  await expect(main.getByRole("button", { name: "批量检查" })).toBeDisabled();
  await expect(main.getByRole("button", { name: "批量清理" })).toBeDisabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("有数据时以 HeroUI 展示统计、筛选、选择和列表操作", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/strm/strm-1", (route) =>
    route.fulfill({
      json: {
        ...first,
        output_path: "/STRM/电影/科幻/流浪地球2.strm",
        file_state: "present",
        content: "http://mediahub.example/p/strm-1/playback-token/movie.mkv\n",
      } satisfies Schemas["StrmDetail"],
    }),
  );
  if (testInfo.project.name === "desktop") {
    await page.setViewportSize({ width: 1440, height: 980 });
  }
  const rowsLoaded = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/strm" && response.ok(),
  );
  await page.goto("/strm");
  await rowsLoaded;
  const main = page.getByRole("main");
  await expect(main.getByText("共 26 条")).toBeVisible();
  for (const label of ["STRM 文件", "有效", "异常", "待处理"]) {
    await expect(main.getByText(label, { exact: true })).toBeVisible();
  }
  await expect(main.getByRole("button", { name: /状态/ })).toBeDisabled();
  await expect(main.getByRole("button", { name: /时间/ })).toBeDisabled();
  await expect(
    main.getByRole("button", { name: "批量重新生成" }),
  ).toBeDisabled();
  const grid = main.getByRole("grid", { name: "STRM 文件列表" });
  await expect(grid.getByRole("columnheader", { name: "操作" })).toBeVisible();
  const selectPage = main.getByRole("checkbox", { name: "选择本页全部文件" });
  await expect(selectPage).toBeVisible();
  await expect(main.getByText(/已选择\s*0\s*项/)).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await main.screenshot({ path: testInfo.outputPath("strm-list.png") });

  const row = grid.getByRole("row", { name: /流浪地球2/ });
  const checkbox = row.getByRole("checkbox");
  await checkbox.focus();
  await page.keyboard.press("Space");
  await expect(checkbox).toBeChecked();
  await expect(selectPage).toHaveJSProperty("indeterminate", true);
  await expect(main.getByText(/已选择\s*1\s*项/)).toBeVisible();
  const downloadEvent = page.waitForEvent("download");
  await main.getByRole("button", { name: "导出列表" }).click();
  const download = await downloadEvent;
  const csv = await readFile(await download.path(), "utf8");
  expect(csv).toContain(first.relative_path);
  expect(csv).not.toContain(data[1]?.relative_path);
  expect(csv).not.toContain("playback-token");

  await row.getByRole("button", { name: /操作/ }).click();
  await page.getByRole("menuitem", { name: "查看详情" }).click();
  await expect(main.getByRole("heading", { name: "文件详情" })).toBeVisible();
});

test("窄屏可翻动多页清单且页面保持视口宽度", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.route("**/api/v1/strm?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    return route.fulfill({
      json: {
        items: data,
        total: 28_560,
        page: Number(search.get("page") ?? 1),
        limit: 50,
      } satisfies Schemas["StrmPage"],
    });
  });
  await page.goto("/strm?page=287");
  const pager = page.getByRole("navigation", { name: "STRM 文件分页" });
  await expect(pager.getByRole("button", { name: "第 572 页" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    320,
  );
  await pager.getByRole("button", { name: "下一页" }).click();
  await expect(page).toHaveURL(/page=288/);
});

test("媒体库全量同步确认后提交任务并关联真实同步历史和按库清单", async ({
  page,
}, testInfo) => {
  let submissions = 0;
  let accepted = false;
  let releaseScan!: () => void;
  const scan = new Promise<void>((resolve) => {
    releaseScan = resolve;
  });
  let releaseSubmission!: () => void;
  const submission = new Promise<void>((resolve) => {
    releaseSubmission = resolve;
  });
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 30, version: "v1.3.0" } }),
  );
  await page.route("**/api/v1/libraries/movie-library", (route) =>
    route.fulfill({ json: library }),
  );
  await page.route(
    "**/api/v1/libraries/movie-library/full-sync-preview",
    async (route) => {
      expect(route.request().method()).toBe("GET");
      await scan;
      await route.fulfill({
        json: {
          revision: library.revision,
          mode: "sample",
          stop_reason: "scope_complete",
          scan_complete: true,
          scanned_videos: 27,
          candidate_count: 26,
          failed_count: 1,
          samples: [
            {
              source_path: first.source_path,
              relative_path: first.relative_path,
            },
          ],
          output: {
            status: "ready",
            message: "服务进程可访问并写入输出目录。",
          },
        } satisfies Schemas["FullSyncPreview"],
      });
    },
  );
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({ json: { items: [capability] } }),
  );
  await page.route("**/api/v1/tasks?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    expect(search.get("type")).toBe("strm");
    expect(search.get("library_id")).toBe(library.id);
    return route.fulfill({
      json: {
        items: accepted ? [completed] : [],
        next_cursor: null,
        total: accepted ? 1 : 0,
        by_type: { strm: accepted ? 1 : 0 },
        by_status: { completed: accepted ? 1 : 0 },
      } satisfies Schemas["TaskPage"],
    });
  });
  await page.route("**/api/v1/tasks", async (route) => {
    const request = route.request();
    expect(request.method()).toBe("POST");
    expect(request.headers()["x-csrf-token"]).toBe("token");
    expect(request.postDataJSON()).toEqual({
      type: "strm",
      library_id: library.id,
    });
    submissions++;
    if (submissions === 1) {
      await route.fulfill({
        status: 503,
        json: {
          error: {
            code: "service_unavailable",
            request_id: "sync-unavailable",
          },
        },
      });
      return;
    }
    await submission;
    accepted = true;
    await route.fulfill({ status: 202, json: completed });
  });
  await page.route("**/api/v1/tasks/generated-task", (route) =>
    route.fulfill({ json: completed }),
  );
  await page.route("**/api/v1/tasks/generated-task/failures?**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: null } }),
  );
  await page.goto("/libraries/movie-library");
  await expect(page.getByRole("button", { name: "生成 STRM" })).toHaveCount(0);
  await page.getByRole("button", { name: "全量同步", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "最近执行记录" })
      .getByText("暂无全量同步记录。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: /下一步.*执行预览/ }).click();
  await expect(page.getByRole("heading", { name: "执行预览" })).toBeVisible();
  await expect(
    page.getByText(library.directory_path, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(library.output_path, { exact: true }),
  ).toBeVisible();
  await expect(page.locator("code")).toContainText(
    `${library.strm_direct_domain}/p/artifact-id/playback-token/`,
  );
  expect(submissions).toBe(0);
  const confirm = page.getByRole("button", { name: "确认执行" });
  await expect(
    page.getByText("正在抽样读取 115 目录并检查输出目录…"),
  ).toBeVisible();
  await expect(confirm).toBeDisabled();
  releaseScan();
  const scanResults = page.getByRole("region", { name: "抽样结果" });
  await expect(scanResults.getByText("抽样发现的视频")).toBeVisible();
  await expect(scanResults.getByText("27", { exact: true })).toBeVisible();
  await expect(scanResults.getByText("26", { exact: true })).toBeVisible();
  await expect(scanResults.getByText("1", { exact: true })).toBeVisible();
  await expect(scanResults.getByText("输出目录可用")).toBeVisible();
  const sampleTable = scanResults.getByRole("grid", { name: "候选文件示例" });
  await expect(sampleTable.getByText(first.source_path)).toBeVisible();
  await expect(sampleTable.getByText(first.relative_path)).toBeVisible();
  await expect(confirm).toBeEnabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("full-sync-scan-results.png"),
    fullPage: true,
  });
  await confirm.click();
  await expect(page.getByRole("alert")).toContainText("sync-unavailable");
  await expect(page).toHaveURL(/\/full-sync\/execution-preview$/);
  await confirm.click();
  await expect(page.getByRole("button", { name: "正在提交…" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "取消" })).toBeDisabled();
  await expect(page).toHaveURL(/\/full-sync\/execution-preview$/);
  releaseSubmission();
  await expect(page).toHaveURL("/tasks/generated-task");
  await expect(
    page.getByRole("heading", { name: "全量同步（STRM）任务" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "结果统计" })).toBeVisible();

  expect(submissions).toBe(2);
  await page.goBack();
  await page.getByRole("link", { name: "返回全量同步" }).click();
  const historyRow = page
    .getByRole("grid", { name: "全量同步历史记录" })
    .getByRole("row")
    .filter({ has: page.getByRole("gridcell", { name: "已完成" }) });
  await expect(historyRow).toContainText("生成 26");
  await expect(
    historyRow.getByRole("link", { name: "查看任务" }),
  ).toHaveAttribute("href", "/tasks/generated-task");
  await expect(
    page.getByRole("link", { name: "查看全部历史" }),
  ).toHaveAttribute("href", "/tasks?type=strm&library_id=movie-library");
  await page.getByRole("link", { name: "返回媒体库详情" }).click();
  const statistics = page.locator('[data-slot="card"]', {
    has: page.getByRole("heading", { name: "STRM 统计" }),
  });
  await expect(statistics.getByText("26", { exact: true })).toHaveCount(2);
  await statistics.getByRole("link", { name: "查看 STRM" }).click();
  await expect(page).toHaveURL("/strm?library_id=movie-library");
  await expect(page.getByText("共 26 条")).toBeVisible();
});

test("登记请求失败可重试，登记文件缺失与列表为空均有说明", async ({ page }) => {
  await page.route("**/api/v1/strm/strm-1", (route) =>
    route.fulfill({
      json: {
        ...first,
        output_path: "/STRM/电影/科幻/流浪地球2.strm",
        file_state: "missing",
        content: null,
      } satisfies Schemas["StrmDetail"],
    }),
  );
  let unavailable = true;
  await page.route("**/api/v1/strm?**", (route) => {
    if (!unavailable) return route.fallback();
    return route.fulfill({
      status: 503,
      json: {
        error: { code: "service_unavailable", request_id: "strm-check" },
      },
    });
  });
  await page.goto("/strm?q=流浪地球");
  await expect(page.getByRole("alert")).toContainText("strm-check");
  unavailable = false;
  await page.getByRole("button", { name: "重新加载", exact: true }).click();
  await page.getByRole("link", { name: first.relative_path }).click();
  await expect(page.getByText("本地文件已缺失，无法预览。")).toBeVisible();
  await page.getByRole("link", { name: "返回 STRM 管理" }).click();
  await page.getByRole("searchbox", { name: "搜索 STRM 文件" }).fill("空结果");
  await expect(page.getByText("没有符合条件的 STRM 文件")).toBeVisible();
});
