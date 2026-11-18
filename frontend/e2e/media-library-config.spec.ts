import { expect, test } from "@playwright/test";

const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "电影库",
  directory_id: "123",
  directory_path: "115 网盘/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/app/media/电影",
  generate_strm: true,
  strm_existing_file_policy: "overwrite",
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

test("配置表单策略保留、格式示例和标签对齐", async ({ page }, testInfo) => {
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
  await page.route("**/api/v1/libraries/1", (route) =>
    route.fulfill({ json: library }),
  );
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 1000,
  });
  await page.goto("/libraries/new");
  const policy = page.getByRole("button", { name: /STRM 已存在时/ });
  await expect(policy).toContainText("跳过（默认）");
  await policy.click();
  await page.getByRole("option", { name: "覆盖", exact: true }).click();
  await page.getByRole("checkbox", { name: "STRM 文件" }).press("Space");
  await expect(policy).toBeDisabled();
  await expect(policy).toContainText("覆盖");
  await page.getByRole("checkbox", { name: "STRM 文件" }).press("Space");
  await expect(policy).toBeEnabled();
  await expect(policy).toContainText("覆盖");
  await expect(
    page.getByText("格式示例：https://media.example.com/电影名称.mkv", {
      exact: true,
    }),
  ).toBeVisible();
  const idTab = page.getByRole("tab", { name: "文件 ID", exact: true });
  await idTab.click();
  await expect(
    page.getByText("格式示例：https://media.example.com/123456789", {
      exact: true,
    }),
  ).toBeVisible();
  const host = `${"media".repeat(12)}.${"example".repeat(8)}.com:8443`;
  await page.getByLabel("STRM 直连域名", { exact: true }).fill(host);
  await expect(
    page.getByText(`格式示例：https://${host}/123456789`, { exact: true }),
  ).toBeVisible();
  await idTab.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(
    page.getByRole("tab", { name: "文件原名", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  const label = await page.locator("#strm-link-format-label").boundingBox();
  const tabs = await page
    .getByRole("tablist", { name: "STRM 链接格式" })
    .boundingBox();
  if (!label || !tabs) throw new Error("格式行未能读取布局尺寸");
  if (testInfo.project.name === "desktop")
    expect(
      Math.abs(label.y + label.height / 2 - tabs.y - tabs.height / 2),
    ).toBeLessThan(2);
  else expect(tabs.y).toBeGreaterThan(label.y);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("config-format.png"),
    fullPage: true,
  });
  await page.goto("/libraries/1/edit");
  await expect(
    page.getByRole("button", { name: /STRM 已存在时/ }),
  ).toContainText("覆盖");
  await page.getByRole("checkbox", { name: "STRM 文件" }).press("Space");
  await expect(
    page.getByRole("button", { name: /STRM 已存在时/ }),
  ).toBeDisabled();
});
