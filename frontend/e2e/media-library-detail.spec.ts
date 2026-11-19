import { expect, test } from "@playwright/test";

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
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 1052880, version: "v1.3.0" } }),
  );
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 1000,
  });
});

test("详情卡片、演示记录和真实启停适配主题与窄屏", async ({
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
  await expect(page.getByText("演示数据", { exact: true })).toHaveCount(3);
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

test("三个同步下级页面支持导航、演示交互和窄屏布局", async ({
  page,
}, testInfo) => {
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
    await expect(
      page.getByText("演示数据", { exact: true }).first(),
    ).toBeVisible();
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
  const history = page.getByRole("grid");
  const initialRows = await history.getByRole("row").count();
  await page.getByRole("button", { name: "查看全部" }).click();
  await expect
    .poll(() => history.getByRole("row").count())
    .toBeGreaterThan(initialRows);
  await history.getByRole("button", { name: "详情" }).first().click();
  await expect(page.getByRole("heading", { name: "执行详情" })).toBeVisible();
  await page.getByRole("button", { name: "返回记录" }).click();
  const preview = page.getByRole("button", { name: /下一步.*执行预览/ });
  await preview.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/libraries\/1\/full-sync\/execution-preview$/);
  await expect(
    page.getByRole("heading", { name: "执行预览", exact: true }),
  ).toBeVisible();
  await capture("execution-preview.png");
  await page.getByText(/^删除\s*[（(]/).click();
  await expect(page).toHaveURL(/change=deleted/);
  await page.reload();
  await expect(
    page.getByRole("grid").getByText("删除", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("grid").getByText("新增", { exact: true }),
  ).toHaveCount(0);
  await page.goBack();
  await expect(
    page.getByRole("grid").getByText("新增", { exact: true }).first(),
  ).toBeVisible();
  await page.goForward();
  await expect(page).toHaveURL(/change=deleted/);
  await page.getByRole("button", { name: "确认执行" }).click();
  await expect(page.getByText("演示执行已完成。")).toBeVisible();
  await page.getByRole("link", { name: "取消" }).click();
  await expect(page).toHaveURL(/\/libraries\/1\/full-sync$/);

  await page.goto("/libraries/1/incremental-sync");
  await expect(
    page.getByRole("heading", { name: "增量同步", exact: true }),
  ).toBeVisible();
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
