import { readFile } from "node:fs/promises";

import { expect, test } from "@playwright/test";

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
});

test("STRM 列表保留跨页选择且导出全部匹配页", async ({ page }, testInfo) => {
  const main = page.getByRole("main");
  await page.goto("/strm");
  await expect(main.getByRole("heading", { name: "STRM 管理" })).toBeVisible();
  await expect(
    page
      .getByRole("complementary", { name: "主导航" })
      .getByRole("link", { name: "STRM 管理" }),
  ).toHaveAttribute("aria-current", "page");
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme",
    testInfo.project.name === "mobile-dark" ? "dark" : "light",
  );
  const file = main.getByRole("row", { name: /流浪地球2/ });
  const checkbox = file.getByRole("checkbox");
  await file.press("Space");
  await expect(checkbox).toBeChecked();
  await main.getByRole("button", { name: "下一页" }).click();
  await expect(main.getByText("已选择 1 项")).toBeVisible();
  await expect(file).toHaveCount(0);
  const downloadPromise = page.waitForEvent("download");
  await main.getByRole("button", { name: "导出列表" }).click();
  const download = await downloadPromise;
  const filePath = await download.path();
  expect(await readFile(filePath, "utf8")).toContain("流浪地球2");
  await main.getByRole("button", { name: "上一页" }).click();
  await expect(checkbox).toBeChecked();

  const search = main.getByRole("searchbox", { name: "搜索 STRM 文件" });
  await search.fill("流浪地球");
  await expect(main.getByText("已选择 0 项")).toBeVisible();
  await expect(main.getByRole("button", { name: "下一页" })).toBeDisabled();
  await file.locator('[data-slot="checkbox-content"]').click();
  await expect(checkbox).toBeChecked();
  await file.getByRole("link").click();
  await expect(main.getByRole("heading", { name: "文件详情" })).toBeVisible();
  await expect(main.locator("pre")).toContainText("#EXTM3U");
  await main.getByRole("button", { name: "定位源文件" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "样例源目录" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(main.getByRole("button", { name: "定位源文件" })).toBeFocused();
  await main.getByRole("link", { name: "返回 STRM 管理" }).click();
  await expect(search).toHaveValue("流浪地球");
  await expect(checkbox).toBeChecked();
  await main.getByRole("button", { name: "重置" }).click();
  await expect(search).toHaveValue("");
  await expect(main.getByText("已选择 0 项")).toBeVisible();
});

test("检查和清理只处理所选演示文件", async ({ page }) => {
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method()))
      mutations.push(request.url());
  });
  const main = page.getByRole("main");
  await page.goto("/strm?size=25");
  const valid = main.getByRole("row", { name: /流浪地球2/ });
  const invalid = main.getByRole("row", { name: /沙丘2/ });
  const pending = main.getByRole("row", { name: /三体.*S01E01/ });
  await valid.locator('[data-slot="checkbox-content"]').click();
  await invalid.locator('[data-slot="checkbox-content"]').click();
  await pending.locator('[data-slot="checkbox-content"]').click();
  await main.getByRole("button", { name: "批量检查" }).click();
  await expect(
    main.getByRole("heading", { name: "有效性检查完成" }),
  ).toBeVisible();
  await expect(main.getByText(/共检查 3 个 STRM 文件/)).toBeVisible();
  await expect(invalid).toContainText("源文件不存在或已被移动");
  await valid.getByRole("link").click();
  await main.getByRole("link", { name: "返回检查结果" }).click();
  await pending.getByRole("button", { name: "重新生成", exact: true }).click();
  await expect(main.getByText("已重新生成 1 个演示文件。")).toBeVisible();
  await expect(pending).toContainText("待处理");
  await main.getByRole("link", { name: "返回 STRM 管理" }).click();
  await expect(page).toHaveURL(/\/strm\?size=25$/);
  await expect(pending).toContainText("有效");
  await expect(main.getByText("已选择 3 项")).toBeVisible();
  await main.getByRole("button", { name: "批量清理" }).click();
  const confirm = main.getByRole("button", { name: "确认清理（1 个文件）" });
  await expect(confirm).toBeEnabled();
  await main.getByRole("button", { name: "取消", exact: true }).click();
  await expect(main.getByText("已选择 3 项")).toBeVisible();
  await main.getByRole("button", { name: "批量清理" }).click();
  await confirm.click();
  await expect(main.getByText("已清理 1 个演示文件。")).toBeVisible();
  await expect(invalid).toHaveCount(0);
  await expect(valid).toBeVisible();
  expect(mutations).toEqual([]);
  await page.reload();
  await expect(invalid).toBeVisible();
  const { scrollWidth, clientWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
});
