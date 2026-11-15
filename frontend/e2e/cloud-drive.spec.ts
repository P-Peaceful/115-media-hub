import { expect, test } from "@playwright/test";

test("网盘浏览区、游标分页及详情弹窗适配亮暗和窄屏", async ({
  page,
}, testInfo) => {
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
  await page.route("**/api/v1/cloud-drive/files?**", (route) => {
    const pageNumber = Number(
      new URL(route.request().url()).searchParams.get("page") ?? "1",
    );
    return route.fulfill({
      json: {
        directory_id: "0",
        path: [{ id: "0", name: "115 网盘" }],
        items:
          pageNumber > 1
            ? []
            : [
                "国产电影",
                "欧美电影",
                "动画电影",
                "电视剧",
                "流浪地球2 (2023).mp4",
                "奥本海默 (2023).mkv",
                "海报.jpg",
                "字幕.srt",
              ].map((name, index) => ({
                id: String(index + 1),
                name,
                is_directory: index < 4,
                size_bytes: index < 4 ? null : "1024",
                modified_at: null,
                file_type:
                  index < 4 ? "directory" : index < 6 ? "video" : "image",
              })),
        total: pageNumber > 1 ? 8 : 51,
        page: pageNumber,
        page_size: 50,
        total_pages: pageNumber > 1 ? 1 : 2,
        next_cursor: pageNumber > 1 ? null : "next",
      },
    });
  });
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 60, version: "dev" } }),
  );
  await page.route("**/health/ready", (route) =>
    route.fulfill({ json: { status: "ok" } }),
  );
  const mobile = testInfo.project.name === "mobile-dark";
  await page.setViewportSize({ width: mobile ? 390 : 1115, height: 901 });
  await page.goto("/cloud-drive");
  await expect(page.getByRole("grid", { name: "网盘目录" })).toBeVisible();
  const pager = page.getByRole("navigation", { name: "目录分页" });
  await expect(pager.getByText("总计 51 项")).toBeVisible();
  await expect(pager.getByText("本页 8 项")).toBeVisible();
  await expect(page.getByText("总计 51 项")).toHaveCount(1);
  await expect(pager.getByRole("button", { name: "第 2 页" })).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "目录路径" }),
  ).toBeVisible();
  await page
    .locator("main")
    .screenshot({ path: testInfo.outputPath("cloud-drive.png") });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const tableWidth = (
    await page.getByRole("grid", { name: "网盘目录" }).boundingBox()
  )?.width;
  await page
    .getByRole("button", { name: "流浪地球2 (2023).mp4", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "文件详情" });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByText("115 网盘 / 流浪地球2 (2023).mp4"),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "跳转媒体库" }),
  ).toBeDisabled();
  expect(
    (
      await page
        .getByRole("grid", { name: "网盘目录", includeHidden: true })
        .boundingBox()
    )?.width,
  ).toBe(tableWidth);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(
    page.getByRole("button", { name: "流浪地球2 (2023).mp4", exact: true }),
  ).toBeFocused();
  const actions = page.getByRole("button", {
    name: "流浪地球2 (2023).mp4 的操作",
  });
  await actions.click();
  await page.getByRole("menuitem", { name: "查看详情" }).click();
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(actions).toBeFocused();
  await page.getByRole("button", { name: "网格视图" }).click();
  await expect(pager.getByText("总计 51 项")).toBeVisible();
  await expect(pager.getByText("本页 8 项")).toBeVisible();
  await expect(page.getByText("总计 51 项")).toHaveCount(1);
  await pager.getByRole("button", { name: "第 2 页" }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText("当前页没有文件")).toBeVisible();
  await expect(pager.getByText("本页 0 项")).toBeVisible();
  await page.goBack();
  await expect(pager.getByText("总计 51 项")).toBeVisible();
  await page.goForward();
  await expect(pager.getByText("本页 0 项")).toBeVisible();
  await page.getByRole("button", { name: "上一页" }).click();
  await expect(
    page.getByRole("button", { name: "国产电影", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 320, height: 901 });
  await page.route("**/api/v1/cloud-drive/files?**", (route) =>
    route.fulfill({
      json: {
        directory_id: "0",
        path: [{ id: "0", name: "115 网盘" }],
        items: [],
        next_cursor: null,
        total: Number.MAX_SAFE_INTEGER,
        page: 1,
        page_size: 50,
        total_pages: Math.ceil(Number.MAX_SAFE_INTEGER / 50),
      },
    }),
  );
  await page.reload();
  const maxTotal = page.getByText("总计 9007199254740991 项");
  await expect(maxTotal).toBeVisible();
  await expect(page.getByText("本页 0 项")).toBeVisible();
  const summaryBounds = await pager.boundingBox();
  const listBounds = await page.locator(".drive-list").boundingBox();
  const totalBounds = await maxTotal.boundingBox();
  const currentBounds = await page.getByText("本页 0 项").boundingBox();
  expect(summaryBounds).not.toBeNull();
  expect(listBounds).not.toBeNull();
  expect(totalBounds).not.toBeNull();
  expect(currentBounds).not.toBeNull();
  expect(summaryBounds!.x).toBeGreaterThanOrEqual(listBounds!.x);
  expect(summaryBounds!.x + summaryBounds!.width).toBeLessThanOrEqual(
    listBounds!.x + listBounds!.width,
  );
  expect(totalBounds!.x + totalBounds!.width).toBeLessThanOrEqual(
    summaryBounds!.x + summaryBounds!.width,
  );
  expect(currentBounds!.x + currentBounds!.width).toBeLessThanOrEqual(
    summaryBounds!.x + summaryBounds!.width,
  );
});

test("慢速目录切换保持路径容器并在失败时显示正确状态", async ({ page }) => {
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
  await page.route("**/api/v1/cloud-drive/files?**", async (route) => {
    const id = new URL(route.request().url()).searchParams.get("directory_id");
    if (id === "123") {
      await new Promise((resolve) => setTimeout(resolve, 500));
      await route.fulfill({
        status: 404,
        json: {
          error: { code: "not_found", message: "missing", request_id: "test" },
        },
      });
    } else {
      await route.fulfill({
        json: {
          directory_id: "0",
          path: [{ id: "0", name: "115 网盘" }],
          items: [
            {
              id: "123",
              name: "电影",
              is_directory: true,
              size_bytes: null,
              modified_at: null,
              file_type: "directory",
            },
          ],
          total: 1,
          page: 1,
          page_size: 50,
          total_pages: 1,
          next_cursor: null,
        },
      });
    }
  });
  await page.goto("/cloud-drive");
  const nav = page.getByRole("navigation", { name: "目录路径" });
  await expect(
    page.getByRole("button", { name: "电影", exact: true }),
  ).toBeVisible();
  const handle = await nav.elementHandle();
  await page.getByRole("button", { name: "电影", exact: true }).click();
  await expect(nav.getByText("115 网盘")).toBeVisible();
  await expect(nav.getByText("电影")).toBeVisible();
  expect(
    await nav.evaluate((element, original) => element === original, handle),
  ).toBe(true);
  await expect(page.getByText("当前目录不存在或已被移除")).toBeVisible();
  await expect(nav.getByText("目录路径暂不可用")).toBeVisible();
  await page.goBack();
  await expect(nav.getByText("115 网盘")).toBeVisible();
  await expect(page.getByText("总计 1 项")).toBeVisible();
});
