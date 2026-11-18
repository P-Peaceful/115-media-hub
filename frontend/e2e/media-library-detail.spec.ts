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

test("详情卡片、演示记录和真实启停适配主题与窄屏", async ({
  page,
}, testInfo) => {
  let enabled = true;
  let longText = false;
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
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 1000,
  });
  await page.goto("/libraries/1");
  await expect(page.getByRole("heading", { name: "库概览" })).toBeVisible();
  await expect(page.getByText("演示数据", { exact: true })).toHaveCount(3);
  await expect(page.getByText("v1.3.0", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "全量同步" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "执行预览" })).toBeDisabled();
  await expect(page.getByRole("link", { name: "编辑配置" })).toHaveAttribute(
    "href",
    "/libraries/1/edit",
  );
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
});
