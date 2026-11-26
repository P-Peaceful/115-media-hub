import { expect, test } from "@playwright/test";

test("媒体库沿用工作台主题且弹窗和输入可辨认", async ({ page }, testInfo) => {
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
  await page.route("**/api/v1/cloud-drive/files?**", (route) =>
    route.fulfill({
      json: {
        directory_id: "0",
        total: 0,
        page: 1,
        page_size: 50,
        total_pages: 1,
        path: [{ id: "0", name: "115 网盘" }],
        items: [],
        next_cursor: null,
      },
    }),
  );
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "1",
            revision: 1,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
            name: "电影库",
            directory_id: "0",
            directory_path: "115 网盘",
            media_type: "movie",
            enabled: true,
            output_path: "/app/media",
            generate_strm: true,
            strm_existing_file_policy: "skip",
            generate_poster_cover: true,
            language_preference: "zh-CN",
            scrape_source: "tmdb",
            exclude_directory_path: "",
            include_subdirectories: true,
            follow_directory_changes: true,
            file_encoding: "UTF-8",
            strm_direct_domain: "",
            strm_link_format: "original_name",
            generate_nfo: true,
            link_type: "direct",
          },
        ],
        next_cursor: null,
      },
    }),
  );
  await page.goto("/libraries/new");
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme",
    testInfo.project.name === "mobile-dark" ? "dark" : "light",
  );
  const themeColor = async (token: string) =>
    page.evaluate((name) => {
      const probe = document.createElement("span");
      probe.style.color = `var(--${name})`;
      document.body.append(probe);
      const color = getComputedStyle(probe).color;
      probe.remove();
      return color;
    }, token);
  const card = page.locator('main [data-slot="card"]').first();
  await expect(card).toHaveCSS("background-color", await themeColor("surface"));
  await expect(page.getByLabel("媒体库名称 *")).toHaveCSS(
    "background-color",
    await themeColor("field-background"),
  );
  await expect(page.getByRole("heading", { name: /新建媒体库/ })).toHaveCSS(
    "color",
    await themeColor("foreground"),
  );
  await page.getByRole("button", { name: /链接类型/ }).click();
  const popover = page.locator('[data-slot="select-popover"]');
  await expect(popover).toBeVisible();
  await expect(popover).toHaveCSS(
    "background-color",
    await themeColor("overlay"),
  );
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "媒体库目录 *", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "选择媒体库目录" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveCSS(
    "background-color",
    await themeColor("overlay"),
  );
  await expect(dialog).toHaveCSS(
    "color",
    await themeColor("overlay-foreground"),
  );
  await expect(
    dialog.getByRole("button", { name: "选择当前目录" }),
  ).toBeEnabled();
  await page.goto("/libraries");
  await page.getByRole("button", { name: "删除" }).click();
  const deleteDialog = page.getByRole("alertdialog", { name: "删除媒体库？" });
  await expect(deleteDialog).toBeVisible();
  await expect(deleteDialog).toHaveCSS(
    "background-color",
    await themeColor("overlay"),
  );
  await expect(deleteDialog).toHaveCSS(
    "color",
    await themeColor("overlay-foreground"),
  );
  await expect(
    deleteDialog.getByRole("button", { name: "确认删除" }),
  ).toBeVisible();
});
