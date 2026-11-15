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
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: null } }),
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
    await expect(page.getByText("115 授权即将失效")).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    const sidebar = page.getByRole("complementary", { name: "主导航" });
    await expect(sidebar).toHaveCSS("position", "static");
    expect((await sidebar.boundingBox())?.height).toBeLessThan(160);
    await expect(sidebar.getByRole("link", { name: "设置" })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "总览" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const cloud = sidebar.getByRole("link", { name: "网盘浏览" });
    await cloud.focus();
    await page.keyboard.press("Tab");
    await expect(sidebar.getByRole("link", { name: "媒体库" })).toBeFocused();
    await sidebar.screenshot({ path: "/tmp/sidebar-mobile-dark.png" });
    await page.screenshot({ path: "/tmp/sidebar-reference-mobile.png" });
    const capacityButton = page.getByRole("button", { name: "容量详情" });
    await expect(capacityButton).toBeVisible();
    await capacityButton.screenshot({ path: "/tmp/capacity-mobile-dark.png" });
    await capacityButton.click();
    await expect(page).toHaveURL(/\/settings\?tab=account$/);
    return;
  }

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/");
  for (const width of [701, 900, 901, 1199, 1200, 1448]) {
    await page.setViewportSize({ width, height: 900 });
    const rows = await page.evaluate(() => {
      const first = Array.from(document.querySelectorAll(".first-row > *"));
      const stats = Array.from(document.querySelectorAll(".stats-row > *"));
      return first.map((card, index) => ({
        first: card.getBoundingClientRect().width,
        stat: stats[index]?.getBoundingClientRect().width ?? 0,
      }));
    });
    for (const row of rows) {
      expect(Math.abs(row.first - row.stat)).toBeLessThan(1);
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    const overflow = await page
      .locator(".first-row .dashboard-card, .stats-row .dashboard-card")
      .evaluateAll((cards) =>
        cards.some((card) => {
          const bounds = card.getBoundingClientRect();
          return Array.from(card.querySelectorAll("h2, strong, button")).some(
            (item) => {
              const itemBounds = item.getBoundingClientRect();
              return (
                itemBounds.left < bounds.left - 1 ||
                itemBounds.right > bounds.right + 1
              );
            },
          );
        }),
      );
    expect(overflow, `card content overflows at ${width}px`).toBe(false);
  }
  await page.setViewportSize({ width: 1448, height: 1086 });
  const alerts = page.locator(".alert-list");
  await expect(alerts.locator(":scope > p")).toHaveCount(3);
  await expect(alerts.getByText("115 授权即将失效")).toBeVisible();
  await expect(alerts.locator("button")).toHaveCount(3);
  for (const button of await alerts.getByRole("button").all()) {
    await expect(button).toBeDisabled();
  }
  const statValues = page.locator(".stats-row .stat-values").first();
  const firstValue = await statValues.locator("strong").nth(0).boundingBox();
  const secondValue = await statValues.locator("strong").nth(1).boundingBox();
  expect(firstValue).not.toBeNull();
  expect(secondValue).not.toBeNull();
  expect(secondValue!.height).toBeLessThanOrEqual(firstValue!.height + 1);
  const exampleHeading = alerts.locator("..").getByRole("heading", {
    name: /异常提醒.*示例数据/,
  });
  await expect(exampleHeading).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  const providerCard = page.getByRole("region", { name: "115 网盘授权" });
  await expect(providerCard.getByText("授权有效")).toBeVisible();
  await expect(providerCard.getByText("138****5678")).toBeVisible();
  await expect(providerCard.getByText("已用 6.57 TB / 13.64 TB")).toBeVisible();
  await expect(
    providerCard.getByRole("progressbar", { name: "网盘空间使用率" }),
  ).toHaveAttribute("aria-valuenow", "48.2");
  await expect(
    providerCard.getByRole("button", { name: "管理授权" }),
  ).toBeVisible();
  await providerCard.screenshot({ path: "/tmp/provider-card-light.png" });
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await providerCard.screenshot({ path: "/tmp/provider-card-dark.png" });
  await page.emulateMedia({ colorScheme: "light" });
  await page.screenshot({ path: "/tmp/dashboard-review-1448.png" });
  await page.setViewportSize({ width: 1280, height: 900 });
  const sidebar = page.getByRole("complementary", { name: "主导航" });
  const settings = sidebar.getByRole("link", { name: "设置" });
  await expect(
    sidebar.getByText("115 Media Hub", { exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 701, height: 900 });
  const brandBounds = await sidebar
    .getByText("115 Media Hub", { exact: true })
    .boundingBox();
  const sidebarBounds = await sidebar.boundingBox();
  expect(brandBounds).not.toBeNull();
  expect(sidebarBounds).not.toBeNull();
  expect(brandBounds!.x + brandBounds!.width).toBeLessThanOrEqual(
    sidebarBounds!.x + sidebarBounds!.width,
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  const breadcrumbs = page.getByRole("navigation", { name: "面包屑" });
  await expect(
    breadcrumbs.getByRole("link", { name: "返回总览" }),
  ).toHaveAttribute("href", "/");
  await expect(breadcrumbs.getByText("工作台")).toBeVisible();
  await expect(breadcrumbs.getByText("总览")).toBeVisible();
  await expect(
    page.locator(".provider-connection-card").getByText("管理授权"),
  ).toHaveCount(0);
  const capacityButton = page.getByRole("button", { name: "容量详情" });
  await expect(capacityButton).toBeVisible();
  await capacityButton.screenshot({ path: "/tmp/capacity-desktop-light.png" });
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await capacityButton.screenshot({ path: "/tmp/capacity-desktop-dark.png" });
  await capacityButton.click();
  await expect(page).toHaveURL(/\/settings\?tab=account$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/$/);
  await page.emulateMedia({ colorScheme: "light" });
  const sidebarManage = sidebar.getByRole("button", { name: /管理授权/ });
  await sidebarManage.click();
  await expect(page).toHaveURL(/\/settings\?tab=account$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/$/);
  await expect(sidebar.getByRole("link", { name: "总览" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(settings).toBeVisible();
  await page.screenshot({ path: "/tmp/sidebar-reference-desktop.png" });
  const cloud = sidebar.getByRole("link", { name: "网盘浏览" });
  const library = sidebar.getByRole("link", { name: "媒体库" });
  await expect(
    sidebar.getByRole("button", { name: "STRM 管理" }),
  ).toBeDisabled();
  await cloud.focus();
  await page.keyboard.press("Tab");
  await expect(library).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/libraries$/);
  await expect(library).toHaveAttribute("aria-current", "page");
  await sidebar.getByRole("link", { name: "总览" }).click();
  await expect(page).toHaveURL(/\/$/);
  const textStart = async (label: string) =>
    sidebar
      .getByRole(label === "STRM 管理" ? "button" : "link", { name: label })
      .evaluate((element) => {
        const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
        const text = walker.nextNode();
        if (!text) throw new Error("菜单文字缺失");
        const range = document.createRange();
        range.selectNodeContents(text);
        return range.getBoundingClientRect().left;
      });
  await expect
    .poll(async () =>
      Math.abs((await textStart("总览")) - (await textStart("STRM 管理"))),
    )
    .toBeLessThan(3);
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
  await settings.focus();
  await expect(settings).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "管理员账号" })).toBeVisible();
  await expect(settings).toHaveAttribute("aria-current", "page");
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
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await sidebar.screenshot({ path: "/tmp/sidebar-desktop-dark.png" });
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await page.setViewportSize({ width: 1448, height: 1086 });
  expect((await sidebar.boundingBox())?.width).toBe(250);
  expect((await page.locator(".app-topbar").boundingBox())?.height).toBe(62);
  const authorizationCard = await sidebar
    .locator(".sidebar-provider-card")
    .boundingBox();
  expect(authorizationCard?.y).toBeGreaterThan(800);
  expect(authorizationCard?.y).toBeLessThan(840);

  await page.setViewportSize({ width: 1280, height: 851 });
  await sidebar.locator(".sidebar-provider-card").scrollIntoViewIfNeeded();
  await expect(sidebar.locator(".sidebar-provider-card")).toBeVisible();
  const settingsCardGap = await sidebar.evaluate((element) => {
    const settings = element.querySelector(
      '.sidebar-bottom a[href="/settings"]',
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
  expect(metrics.overflow).toBe("auto");
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
    const item = sidebar.getByText(label, { exact: true });
    await item.scrollIntoViewIfNeeded();
    await expect(item).toBeInViewport();
  }
  await page.screenshot({ path: "/tmp/sidebar-short.png" });
  await page.setViewportSize({ width: 1280, height: 400 });
  const shortMetrics = await sidebar.evaluate((element) => ({
    height: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(shortMetrics.height).toBe(400);
  await settings.scrollIntoViewIfNeeded();
  const settingsBounds = await settings.boundingBox();
  expect(settingsBounds).not.toBeNull();
  expect(settingsBounds!.y + settingsBounds!.height).toBeLessThanOrEqual(400);
  await page.setViewportSize({ width: 1448, height: 1086 });
  for (const status of ["invalid", "unconfigured"] as const) {
    await page.route("**/api/v1/settings/115/details", (route) =>
      route.fulfill({
        json: {
          status,
          profile: null,
          space: null,
          space_status: "unavailable",
          devices: [],
          devices_status: "unavailable",
        },
      }),
    );
    await page.reload();
    await expect(
      sidebar.getByText(status === "invalid" ? "授权失效" : "尚未授权", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      sidebar.getByRole("button", {
        name: status === "invalid" ? "重新授权" : "管理授权",
      }),
    ).toBeVisible();
    await expect(sidebar.getByRole("progressbar")).toHaveCount(0);
  }
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({ status: 503, json: { message: "unavailable" } }),
  );
  await page.reload();
  await expect(
    sidebar.getByText("无法确认授权", { exact: true }),
  ).toBeVisible();
  await expect(
    sidebar.getByText("容量暂不可用", { exact: true }),
  ).toBeVisible();
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
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({
      json: {
        status: "not_configured",
        profile: null,
        space: null,
        space_status: "unavailable",
        devices: [],
        devices_status: "unavailable",
      },
    }),
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

test("各模块的导航栏与面包屑背景一致，并与内容区区分", async ({ page }) => {
  test.setTimeout(90_000);
  await page.route("**/api/v1/**", (route) => route.fulfill({ json: {} }));
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
  await page.route("**/health/ready", (route) =>
    route.fulfill({ json: { status: "ok" } }),
  );
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    for (const theme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme: theme });
      for (const path of ["/", "/cloud-drive", "/libraries", "/settings"]) {
        await page.goto(path, { waitUntil: "domcontentloaded" });
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await expect(page.locator(".app-sidebar")).toBeVisible();
        const colors = await page.evaluate(() => {
          const getColor = (selector: string) => {
            const element = document.querySelector(selector);
            if (!element) throw new Error(`未找到 ${selector}`);
            return getComputedStyle(element).backgroundColor;
          };
          return {
            sidebar: getColor(".app-sidebar"),
            topbar: getColor(".app-topbar"),
            content: getColor(".app-main"),
          };
        });
        const visibleColors = await page.evaluate(() => {
          const sidebar = document
            .querySelector(".app-sidebar")!
            .getBoundingClientRect();
          const topbar = document
            .querySelector(".app-topbar")!
            .getBoundingClientRect();
          const main = document
            .querySelector(".app-main")!
            .getBoundingClientRect();
          const at = (x: number, y: number) => {
            let element = document.elementFromPoint(x, y);
            while (element) {
              const color = getComputedStyle(element).backgroundColor;
              if (color !== "rgba(0, 0, 0, 0)" && color !== "transparent")
                return color;
              element = element.parentElement;
            }
            throw new Error("未找到可见背景");
          };
          return {
            sidebar: at(sidebar.left + 4, sidebar.top + 4),
            topbar: at(topbar.left + 4, topbar.top + 4),
            content: at(main.left + 4, topbar.bottom + 4),
          };
        });
        expect(colors.sidebar, `${path}, ${theme}, ${width}px`).toBe(
          colors.topbar,
        );
        expect(colors.content, `${path}, ${theme}, ${width}px`).not.toBe(
          colors.topbar,
        );
        expect(visibleColors.sidebar, `${path}, ${theme}, ${width}px`).toBe(
          colors.sidebar,
        );
        expect(visibleColors.topbar, `${path}, ${theme}, ${width}px`).toBe(
          colors.topbar,
        );
        expect(visibleColors.content, `${path}, ${theme}, ${width}px`).toBe(
          colors.content,
        );
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
      }
    }
  }
});
