import { expect, test } from "@playwright/test";

import type { components } from "../src/lib/api/generated/schema";

const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "电影库",
  directory_id: "123",
  directory_path: "/我的资源/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/STRM/电影",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_nfo: true,
  generate_poster_cover: true,
  language_preference: "zh-CN",
  scrape_source: "tmdb",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: true,
  file_encoding: "UTF-8",
  link_type: "proxy",
  strm_direct_domain: "",
  strm_link_format: "original_name",
};

test.beforeEach(async ({ page }, testInfo) => {
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
  await page.route("**/api/v1/strm?**", (route) =>
    route.fulfill({ json: { items: [], total: 0, page: 1, limit: 1 } }),
  );
  await page.route("**/api/v1/tasks?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    expect(search.get("type")).toBe("strm");
    expect(search.get("library_id")).toBe(library.id);
    return route.fulfill({
      json: {
        items: [],
        next_cursor: null,
        total: 0,
        by_type: {},
        by_status: {},
      },
    });
  });
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            type: "strm",
            available: false,
            reason: "未配置播放地址",
            can_pause: false,
            can_retry_failed: false,
            can_schedule: false,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 1052880, version: "v1.3.0" } }),
  );
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 1000,
  });
});

test("执行预览展示输出目录问题并阻止提交，重新扫描后可恢复", async ({
  page,
}) => {
  let revision = library.revision;
  let requests = 0;
  await page.route("**/api/v1/libraries/1", (route) =>
    route.fulfill({
      json: { ...library, revision, strm_direct_domain: "media.example.com" },
    }),
  );
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            type: "strm",
            available: true,
            reason: null,
            can_pause: false,
            can_retry_failed: false,
            can_schedule: false,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/libraries/1/full-sync-preview", (route) => {
    requests++;
    if (requests === 2) {
      return route.fulfill({
        status: 409,
        json: {
          error: {
            code: "provider_auth_invalid",
            request_id: "invalid-115-auth",
          },
        },
      });
    }
    return route.fulfill({
      json: {
        revision: requests === 3 ? revision + 1 : revision,
        scan_complete: requests <= 3,
        scanned_videos: 0,
        candidate_count: 0,
        failed_count: 0,
        samples: [],
        output:
          requests === 1
            ? {
                status: "permission_denied",
                message: "请确认运行服务的用户可写入此目录。",
              }
            : { status: "will_create", message: "同步任务会创建该输出目录。" },
      } satisfies components["schemas"]["FullSyncPreview"],
    });
  });
  await page.goto("/libraries/1/full-sync/execution-preview");
  const confirm = page.getByRole("button", { name: "确认执行" });
  const results = page.getByRole("region", { name: "扫描结果" });
  await expect(results.getByRole("alert")).toContainText("输出目录权限不足");
  await expect(
    results.getByText("请确认运行服务的用户可写入此目录。"),
  ).toBeVisible();
  await expect(confirm).toBeDisabled();

  await results.getByRole("button", { name: "重新扫描" }).click();
  await expect(
    results.getByRole("alert").filter({ hasText: "invalid-115-auth" }),
  ).toBeVisible();
  await expect(
    results.getByRole("link", { name: "管理 115 网盘授权" }),
  ).toHaveAttribute("href", "/settings?tab=account");
  await expect(confirm).toBeDisabled();

  await results.getByRole("button", { name: "重新扫描" }).click();
  await expect(
    results.getByText("媒体库配置已更新，请重新加载后扫描。"),
  ).toBeVisible();
  await expect(confirm).toBeDisabled();
  revision++;
  await results.getByRole("link", { name: "重新加载配置" }).click();
  await expect(results.getByText("部分扫描结果")).toBeVisible();
  await expect(
    results.getByText("未扫描到可生成 STRM 的视频文件。"),
  ).toHaveCount(0);
  await expect(results.getByText("同步时创建输出目录")).toBeVisible();
  await expect(confirm).toBeEnabled();
  expect(requests).toBe(4);
});

test("详情卡片、同步记录空状态和真实启停适配主题与窄屏", async ({
  page,
}, testInfo) => {
  let enabled = true;
  let longText = false;
  await page.route("**/api/v1/libraries/1", (route) =>
    route.fulfill({
      json: {
        ...library,
        enabled,
        ...(longText
          ? {
              name: "Library".repeat(16),
              directory_path: `/电影/${"long-path".repeat(55)}`,
              output_path: `/app/media/${"output".repeat(60)}`,
            }
          : {}),
      },
    }),
  );
  await page.route("**/api/v1/libraries/1/enabled", async (route) => {
    enabled = false;
    await route.fulfill({ json: { ...library, enabled } });
  });
  await page.goto("/libraries/1");
  await expect(page.getByRole("heading", { name: "库概览" })).toBeVisible();
  await expect(page.getByText("演示数据", { exact: true })).toHaveCount(0);
  await expect(
    page.getByText("暂无全量同步记录。", { exact: true }),
  ).toHaveCount(2);
  await expect(page.getByText("服务可访问", { exact: true })).toBeVisible();
  await expect(page.getByText("版本信息", { exact: true })).toHaveCount(0);
  await expect(page.getByText("v1.3.0", { exact: true })).toHaveCount(0);
  const actionButtons = page.getByRole("button", {
    name: /^(全量同步|增量同步|编辑配置)$/,
  });
  await expect(actionButtons).toHaveCount(3);
  const actionMetrics = await actionButtons.evaluateAll((buttons) =>
    buttons.map((button) => {
      const style = getComputedStyle(button);
      const icon = button.querySelector("svg")?.getBoundingClientRect();
      return {
        height: button.getBoundingClientRect().height,
        backgroundColor: style.backgroundColor,
        color: style.color,
        fontSize: style.fontSize,
        fontWeight: style.fontWeight,
        textDecoration: style.textDecorationLine,
        iconWidth: icon?.width,
        iconHeight: icon?.height,
      };
    }),
  );
  expect(actionMetrics).toEqual(Array(3).fill(actionMetrics[0]));
  expect(actionMetrics[0]).toMatchObject({
    textDecoration: "none",
    iconWidth: 20,
    iconHeight: 20,
  });
  await expect(
    page.getByRole("link", { name: library.directory_path }),
  ).toHaveAttribute("href", "/cloud-drive?directory_id=123");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const overview = await page
    .getByRole("heading", { name: "库概览" })
    .boundingBox();
  const status = await page
    .getByRole("heading", { name: "当前状态" })
    .boundingBox();
  expect(overview).not.toBeNull();
  expect(status).not.toBeNull();
  if (overview && status) {
    if (testInfo.project.name === "desktop") expect(status.y).toBe(overview.y);
    else expect(status.y).toBeGreaterThan(overview.y);
  }
  const name = await page
    .getByRole("heading", { name: library.name, level: 3 })
    .boundingBox();
  const language = await page
    .getByText("语言偏好", { exact: true })
    .boundingBox();
  const scraper = await page.getByText("刮削源", { exact: true }).boundingBox();
  if (!name || !language || !scraper) throw new Error("库概览布局缺失");
  if (testInfo.project.name === "desktop") {
    expect(language.x).toBeGreaterThan(name.x + name.width);
    expect(scraper.x).toBe(language.x);
    expect(language.y).toBeLessThan(name.y + name.height);
  } else {
    expect(language.y).toBeGreaterThan(name.y + name.height);
  }
  await page.screenshot({
    path: testInfo.outputPath("detail.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "暂停媒体库" }).click();
  await expect(page.getByText("已暂停", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "启用媒体库" })).toBeEnabled();
  longText = true;
  await page.reload();
  await expect(
    page
      .getByRole("heading", { name: "Library".repeat(16), exact: true })
      .first(),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("detail-long-text.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "编辑配置", exact: true }).click();
  await expect(page).toHaveURL(/\/libraries\/1\/edit$/);
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page).toHaveURL(/\/libraries\/1$/);
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({ json: { items: [library], next_cursor: null } }),
  );
  await page.goto("/libraries?filter=enabled");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  await expect(page).toHaveURL(/\/libraries\/1\/edit$/);
  await page.reload();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page).toHaveURL(/\/libraries\?filter=enabled$/);
  await expect(page.getByRole("radio", { name: /已启用/ })).toBeChecked();
});

test("详情统计与执行记录摘要展示当前媒体库的数据", async ({
  page,
}, testInfo) => {
  const syncTask: components["schemas"]["Task"] = {
    id: "latest-sync",
    type: "strm",
    library_id: library.id,
    library_name: library.name,
    source: "manual",
    schedule_id: null,
    parent_id: null,
    status: "completed",
    outcome: "success",
    created_at: "2026-11-25T02:20:00Z",
    started_at: "2026-11-25T02:24:00Z",
    finished_at: "2026-11-25T02:26:17Z",
    duration_ms: 137_000,
    progress_percent: 100,
    speed_bytes_per_second: null,
    current_file: null,
    total_items: 131,
    succeeded_items: 128,
    failed_items: 0,
    skipped_items: 3,
    actions: [],
  };
  const records = [
    syncTask,
    {
      ...syncTask,
      id: "prior-sync",
      outcome: "partial_failure",
      created_at: "2026-11-24T04:00:00Z",
      started_at: "2026-11-24T04:00:00Z",
      finished_at: "2026-11-24T04:01:03Z",
      duration_ms: 63_000,
      total_items: 57,
      succeeded_items: 56,
      failed_items: 1,
      skipped_items: 0,
    },
    {
      ...syncTask,
      id: "older-sync",
      created_at: "2026-11-23T03:00:00Z",
      started_at: "2026-11-23T03:00:00Z",
      finished_at: "2026-11-23T03:02:17Z",
    },
    {
      ...syncTask,
      id: "oldest-sync",
      status: "failed",
      outcome: "failure",
      created_at: "2026-11-21T03:00:00Z",
      started_at: "2026-11-21T03:00:00Z",
      finished_at: "2026-11-21T03:02:17Z",
      succeeded_items: 0,
      failed_items: 131,
      skipped_items: 0,
    },
  ] satisfies components["schemas"]["Task"][];
  await page.clock.setFixedTime(new Date("2026-11-25T08:00:00Z"));
  await page.route("**/api/v1/libraries/1", (route) =>
    route.fulfill({ json: library }),
  );
  await page.route("**/api/v1/strm?**", (route) => {
    expect(new URL(route.request().url()).searchParams.get("library_id")).toBe(
      library.id,
    );
    return route.fulfill({
      json: { items: [], total: 12_532, page: 1, limit: 1 },
    });
  });
  await page.route("**/api/v1/tasks?**", (route) => {
    const search = new URL(route.request().url()).searchParams;
    expect(search.get("library_id")).toBe(library.id);
    expect(search.get("type")).toBe("strm");
    return route.fulfill({
      json: {
        items: records,
        next_cursor: null,
        total: records.length,
        by_type: { strm: records.length },
        by_status: { completed: 3, failed: 1 },
      } satisfies components["schemas"]["TaskPage"],
    });
  });

  await page.goto("/libraries/1");
  const statistics = page.locator('[data-slot="card"]', {
    has: page.getByRole("heading", { name: "STRM 统计", exact: true }),
  });
  const timeline = page.locator('[data-slot="card"]', {
    has: page.getByRole("heading", { name: "执行记录摘要", exact: true }),
  });
  await expect(statistics.locator("dt")).toHaveText([
    "总数量",
    "已生成",
    "待处理",
    "失败",
  ]);
  await expect(statistics.locator("dd")).toHaveText([
    "12,532",
    "12,532",
    "—",
    "—",
  ]);
  await expect(
    statistics.getByRole("link", { name: "查看 STRM" }),
  ).toHaveAttribute("href", "/strm?library_id=1");
  await expect(
    timeline.getByRole("link", { name: "查看全部" }),
  ).toHaveAttribute("href", "/tasks?type=strm&library_id=1");
  const rows = timeline.getByRole("listitem");
  await expect(rows).toHaveCount(4);
  await expect(rows.first().locator("time")).toHaveText("今天 10:24");
  await expect(rows.nth(1).locator("time")).toHaveText("昨天 12:00");
  await expect(rows.nth(2).locator("time")).toHaveText("11-23 11:00");
  await expect(rows.first()).toContainText("全量同步");
  await expect(rows.first()).toContainText(/生成\s*128/);
  await expect(rows.first()).toContainText(/跳过\s*3/);
  await expect(rows.first()).toContainText("耗时 2 分 17 秒");
  await expect(rows.first()).toContainText("成功");
  await expect(rows.first().getByRole("link")).toHaveAttribute(
    "href",
    "/tasks/latest-sync",
  );
  await expect(rows.nth(1)).toContainText(/失败\s*1/);
  await expect(rows.nth(1)).toContainText("已完成（部分失败）");
  await expect(rows.last().getByText("失败", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await statistics.screenshot({ path: testInfo.outputPath("statistics.png") });
  await timeline.screenshot({ path: testInfo.outputPath("timeline.png") });
});

test("同步页面支持真实配置预览、导航和窄屏布局", async ({ page }, testInfo) => {
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/") && request.method() !== "GET") {
      writes.push(request.method() + " " + new URL(request.url()).pathname);
    }
  });
  await page.route("**/api/v1/libraries/1", (route) =>
    route.fulfill({ json: library }),
  );
  await page.route("**/api/v1/libraries/2", (route) =>
    route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: "媒体库不存在" } },
    }),
  );

  async function capture(name: string) {
    const banner = page.locator("main .page-banner");
    const artwork = banner.locator("img");
    if (testInfo.project.name === "desktop") {
      await expect(artwork).toBeVisible();
      await expect(artwork).toHaveCSS("object-fit", "contain");
      const fits = await artwork.evaluate((image) => {
        if (!(image instanceof HTMLImageElement)) return false;
        const frame = image.parentElement?.getBoundingClientRect();
        const bounds = image.getBoundingClientRect();
        const description = image.parentElement
          ?.querySelector("p")
          ?.getBoundingClientRect();
        return (
          image.naturalWidth > 0 &&
          frame &&
          description &&
          bounds.top >= frame.top &&
          bounds.bottom <= frame.bottom &&
          bounds.right <= frame.right &&
          bounds.left >= description.right
        );
      });
      expect(fits).toBe(true);
    } else {
      await expect(artwork).toBeHidden();
    }
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(await page.evaluate(() => innerWidth));
    await page.screenshot({ path: testInfo.outputPath(name), fullPage: true });
  }

  await page.goto("/libraries/1");
  await expect(
    page.getByRole("button", { name: "执行预览", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "全量同步", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "全量同步", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(library.output_path, { exact: true }),
  ).toBeVisible();
  await capture("full-sync.png");
  await expect(
    page.getByText("暂无全量同步记录。", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("演示数据", { exact: true })).toHaveCount(0);
  const preview = page.getByRole("button", { name: /下一步.*执行预览/ });
  await preview.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/libraries\/1\/full-sync\/execution-preview$/);
  await expect(
    page.getByRole("heading", { name: "执行预览", exact: true }),
  ).toBeVisible();
  await capture("execution-preview.png");
  await expect(page.getByText("演示数据", { exact: true })).toHaveCount(0);
  await expect(page.getByText("STRM 播放地址", { exact: true })).toBeVisible();
  await expect(
    page.getByText("请先在媒体库配置中填写 STRM 播放地址。"),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "确认执行" })).toBeDisabled();
  await expect(
    page.getByRole("link", { name: "编辑媒体库配置" }),
  ).toHaveAttribute("href", "/libraries/1/edit");
  await page.reload();
  await expect(page.getByRole("button", { name: "确认执行" })).toBeDisabled();
  await page.getByRole("link", { name: "取消" }).click();
  await expect(page).toHaveURL(/\/libraries\/1\/full-sync$/);

  await page.goto("/libraries/1/incremental-sync");
  await expect(
    page.getByRole("heading", { name: "增量同步", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("演示数据", { exact: true })).toBeVisible();
  await capture("incremental-sync.png");
  const interval = page.getByRole("button", { name: /轮询间隔/ });
  await interval.click();
  await page.getByRole("option", { name: "60 秒" }).click();
  await expect(interval).toContainText("60 秒");
  const batchLimit = page.getByRole("button", { name: /批量处理上限/ });
  await batchLimit.click();
  await page.getByRole("option", { name: "250 条" }).click();
  await expect(batchLimit).toContainText("250 条");
  const listener = page.getByRole("switch", { name: "启用增量监听" });
  await expect(listener).toBeChecked();
  await page.getByText("启用增量监听", { exact: true }).click();
  await expect(listener).not.toBeChecked();
  await page.getByRole("button", { name: "去处理" }).first().click();
  await expect(page.getByRole("heading", { name: "异常详情" })).toBeVisible();
  await page.getByRole("button", { name: "返回异常列表" }).click();
  await page.getByRole("link", { name: "返回媒体库详情" }).click();
  await page.getByRole("button", { name: "增量同步", exact: true }).click();
  await expect(listener).not.toBeChecked();
  await expect(interval).toContainText("60 秒");
  await expect(batchLimit).toContainText("250 条");
  await page.reload();
  await expect(listener).toBeChecked();
  await expect(interval).toContainText("30 秒");
  await expect(batchLimit).toContainText("500 条");

  await page.goto("/libraries/2/full-sync");
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByRole("link", { name: "返回媒体库" })).toHaveAttribute(
    "href",
    "/libraries",
  );
  expect(writes).toEqual([]);
});
