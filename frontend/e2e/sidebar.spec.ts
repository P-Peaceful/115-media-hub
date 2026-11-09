import { expect, test } from "@playwright/test";

test("全局导航在桌面保持视口定位，短屏完整显示，移动端保留原布局", async ({
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
  await page.route("**/api/v1/settings/security", (route) =>
    route.fulfill({ json: { username: "admin", session_lifetime_days: 7 } }),
  );
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 62, version: "dev" } }),
  );
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({
      json: {
        status: "valid",
        profile: {
          user_name: "个人账号",
          user_id: "13812345678",
          avatar: null,
          is_member: true,
          member_expires_at: null,
          member_forever: false,
        },
        space: { used_bytes: "7230000000000", total_bytes: "15000000000000" },
        space_status: "available",
        devices: [],
        devices_status: "available",
      },
    }),
  );
  await page.route("**/health/ready", (route) =>
    route.fulfill({ json: { status: "ok" } }),
  );

  if (testInfo.project.name === "mobile-dark") {
    await page.goto("/");
    const sidebar = page.getByRole("complementary", { name: "主导航" });
    await expect(sidebar).toHaveCSS("position", "static");
    await expect(sidebar.getByRole("link", { name: "设置" })).toBeVisible();
    return;
  }

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/");
  const sidebar = page.getByRole("complementary", { name: "主导航" });
  const settings = sidebar.getByRole("link", { name: "设置" });
  await expect(settings).toBeVisible();
  expect(
    await sidebar.evaluate(
      (element) => element.scrollHeight <= element.clientHeight,
    ),
  ).toBe(true);
  const gaps = await sidebar.locator("nav").evaluate((nav) => {
    const groups = Array.from(nav.children, (child) =>
      child.getBoundingClientRect(),
    );
    return groups
      .slice(1)
      .map((group, index) => group.top - (groups[index]?.bottom ?? 0));
  });
  expect(Math.min(...gaps.slice(0, -1))).toBeGreaterThanOrEqual(14);
  expect(Math.max(...gaps.slice(0, -1))).toBeLessThanOrEqual(26);
  const bottom = sidebar.locator(".sidebar-bottom");
  const bottomInset = await sidebar.evaluate((element) => {
    const bottom = element.querySelector(".sidebar-bottom");
    return (
      element.getBoundingClientRect().bottom -
      (bottom?.getBoundingClientRect().bottom ?? 0)
    );
  });
  expect(bottomInset).toBeLessThanOrEqual(20);
  await expect(bottom).toBeVisible();
  const overviewWidth = await page
    .locator(".app-topbar")
    .evaluate((element) => element.getBoundingClientRect().width);
  const overviewY = await settings.evaluate(
    (element) => element.getBoundingClientRect().top,
  );
  await settings.click();
  await expect(page.getByRole("heading", { name: "管理员账号" })).toBeVisible();
  const settingsY = await settings.evaluate(
    (element) => element.getBoundingClientRect().top,
  );
  expect(Math.abs(settingsY - overviewY)).toBeLessThan(1);
  const settingsWidth = await page
    .locator(".app-topbar")
    .evaluate((element) => element.getBoundingClientRect().width);
  expect(Math.abs(settingsWidth - overviewWidth)).toBeLessThan(1);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  expect(
    Math.abs(
      await sidebar.evaluate((element) => element.getBoundingClientRect().top),
    ),
  ).toBeLessThanOrEqual(1);
  await page.screenshot({ path: "/tmp/sidebar-desktop.png" });

  await page.setViewportSize({ width: 1280, height: 851 });
  expect(
    await sidebar.evaluate(
      (element) => element.scrollHeight <= element.clientHeight,
    ),
  ).toBe(true);
  await expect(sidebar.locator(".sidebar-provider-card")).toBeVisible();
  const settingsCardGap = await sidebar.evaluate((element) => {
    const settings = element.querySelector(
      '.sidebar-bottom > a[href="/settings"]',
    );
    const card = element.querySelector(".sidebar-provider-card");
    return (
      (card?.getBoundingClientRect().top ?? 0) -
      (settings?.getBoundingClientRect().bottom ?? 0)
    );
  });
  expect(settingsCardGap).toBeGreaterThanOrEqual(12);
  await expect(sidebar.getByText("账号：138****5678")).toBeVisible();
  await expect(sidebar.getByText("已用 6.57 TB / 13.64 TB")).toBeVisible();
  await expect(
    sidebar.getByRole("progressbar", { name: "网盘空间使用率" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 1280, height: 850 });
  await expect(sidebar.locator(".sidebar-provider-card")).toBeHidden();
  expect(
    await sidebar.evaluate(
      (element) => element.scrollHeight <= element.clientHeight,
    ),
  ).toBe(true);
  await expect(settings).toBeInViewport();
  await page.setViewportSize({ width: 1280, height: 500 });
  await page.evaluate(() => window.scrollTo(0, 0));
  const metrics = await sidebar.evaluate((element) => ({
    height: element.clientHeight,
    scrollHeight: element.scrollHeight,
    overflow: getComputedStyle(element).overflowY,
  }));
  expect(metrics.height).toBe(500);
  expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.height);
  expect(metrics.overflow).toBe("visible");
  for (const label of [
    "总览",
    "网盘浏览",
    "媒体库",
    "STRM 管理",
    "海报墙",
    "任务中心",
    "文件回传",
    "运行日志",
    "设置",
  ]) {
    await expect(sidebar.getByText(label, { exact: true })).toBeInViewport();
  }
  await page.screenshot({ path: "/tmp/sidebar-short.png" });
  await page.setViewportSize({ width: 1280, height: 400 });
  const shortMetrics = await sidebar.evaluate((element) => ({
    height: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(shortMetrics.scrollHeight).toBeLessThanOrEqual(shortMetrics.height);
  const settingsBounds = await settings.boundingBox();
  expect(settingsBounds).not.toBeNull();
  expect(settingsBounds!.y + settingsBounds!.height).toBeLessThanOrEqual(400);
});

test("桌面顶部栏在页面滚动状态变化时保持宽度", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
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
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 62, version: "dev" } }),
  );
  await page.route("**/health/ready", (route) =>
    route.fulfill({ json: { status: "ok" } }),
  );
  await page.setViewportSize({ width: 1280, height: 1200 });
  await page.goto("/");
  const topbar = page.locator(".app-topbar");
  await expect(page.getByRole("heading", { name: /欢迎/ })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollHeight <= window.innerHeight,
    ),
  ).toBe(true);
  const shortWidth = await topbar.evaluate(
    (element) => element.getBoundingClientRect().width,
  );
  await page.evaluate(() => {
    const extension = document.createElement("div");
    extension.id = "test-page-extension";
    extension.style.height = "150vh";
    document.querySelector(".app-main")?.append(extension);
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollHeight > window.innerHeight,
    ),
  ).toBe(true);
  const longWidth = await topbar.evaluate(
    (element) => element.getBoundingClientRect().width,
  );
  expect(Math.abs(longWidth - shortWidth)).toBeLessThan(1);
  expect(
    await page
      .locator("html")
      .evaluate((element) => getComputedStyle(element).scrollbarGutter),
  ).toBe("stable");
});
