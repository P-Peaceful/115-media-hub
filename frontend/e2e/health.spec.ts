import { expect, test } from "@playwright/test";

test("检查状态、键盘重试和窄屏布局", async ({ page }) => {
  await page.route("**/api/v1/auth/bootstrap", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        initialized: true,
        authenticated: true,
        administrator: { id: "test-admin", username: "admin" },
        expires_at: "2099-01-01T00:00:00Z",
        csrf_token: "test-csrf",
      }),
    });
  });
  await page.route("**/api/v1/system/status", async (route) => {
    await route.fulfill({ json: { uptime_seconds: 62, version: "dev" } });
  });
  let ready = false;
  await page.route("**/health/ready", async (route) => {
    await route.fulfill({
      status: ready ? 200 : 503,
      contentType: "application/json",
      body: JSON.stringify(
        ready
          ? { status: "ok" }
          : {
              error: {
                code: "service_unavailable",
                message: "Service unavailable",
                request_id: "browser-smoke",
              },
            },
      ),
    });
  });
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "欢迎使用 115 Media Hub",
  );
  const themeToggle = page.getByRole("button", { name: /切换到.*主题/ });
  const previousTheme = await page.locator("html").getAttribute("data-theme");
  await themeToggle.click();
  await expect(page.locator("html")).not.toHaveAttribute(
    "data-theme",
    previousTheme ?? "",
  );
  await expect(themeToggle).toBeVisible();
  await expect(page.getByRole("status")).toContainText("服务暂未就绪");
  ready = true;
  await page.getByRole("button", { name: "重新检查" }).focus();
  await expect(page.getByRole("button", { name: "重新检查" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status")).toContainText(
    "后端服务与数据库连接正常。",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.setViewportSize({ width: 701, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await expect(themeToggle).toBeVisible();
});

test("系统状态与 115 连接卡的徽标和详情行对齐", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.route("**/api/v1/auth/bootstrap", async (route) => {
    await route.fulfill({
      json: {
        initialized: true,
        authenticated: true,
        administrator: { id: "test-admin", username: "admin" },
        expires_at: "2099-01-01T00:00:00Z",
        csrf_token: "test-csrf",
      },
    });
  });
  await page.route("**/api/v1/system/status", async (route) => {
    await route.fulfill({ json: { uptime_seconds: 62, version: "dev" } });
  });
  await page.route("**/health/ready", async (route) => {
    await route.fulfill({ json: { status: "ok" } });
  });
  await page.route("**/api/v1/settings/115/details", async (route) => {
    await route.fulfill({
      json: {
        status: "valid",
        profile: {
          user_name: "test-account",
          user_id: "1234567890",
          avatar: null,
          is_member: true,
          member_expires_at: null,
          member_forever: false,
        },
        space: null,
        space_status: "unavailable",
        devices: [],
        devices_status: "unavailable",
      },
    });
  });

  await page.goto("/");
  const systemCard = page.locator(".system-card");
  const accountCard = page.locator(".provider-connection-card");
  const systemBadge = systemCard.locator(".dashboard-status-badge");
  const accountBadge = accountCard.locator(".dashboard-status-badge");
  const systemDetail = systemCard.locator(".system-line").first();
  const accountDetail = accountCard.locator(".provider-detail-line").first();

  await expect(systemBadge).toBeVisible();
  await expect(accountBadge).toBeVisible();
  const badgeBounds = await Promise.all([
    systemBadge.boundingBox(),
    accountBadge.boundingBox(),
  ]);
  const [systemBadgeBounds, accountBadgeBounds] = badgeBounds;
  if (!systemBadgeBounds || !accountBadgeBounds) {
    throw new Error("状态徽标未能获取布局尺寸");
  }
  expect(Math.abs(systemBadgeBounds.y - accountBadgeBounds.y)).toBeLessThan(1);
  expect(
    Math.abs(systemBadgeBounds.height - accountBadgeBounds.height),
  ).toBeLessThan(1);

  const detailBounds = await Promise.all([
    systemDetail.boundingBox(),
    accountDetail.boundingBox(),
  ]);
  const [systemDetailBounds, accountDetailBounds] = detailBounds;
  if (!systemDetailBounds || !accountDetailBounds) {
    throw new Error("状态详情行未能获取布局尺寸");
  }
  expect(Math.abs(systemDetailBounds.y - accountDetailBounds.y)).toBeLessThan(
    1,
  );

  await page.setViewportSize({ width: 701, height: 900 });
  const narrowDetailBounds = await Promise.all([
    systemDetail.boundingBox(),
    accountDetail.boundingBox(),
  ]);
  const [narrowSystemDetailBounds, narrowAccountDetailBounds] =
    narrowDetailBounds;
  if (!narrowSystemDetailBounds || !narrowAccountDetailBounds) {
    throw new Error("窄屏状态详情行未能获取布局尺寸");
  }
  expect(
    Math.abs(narrowSystemDetailBounds.y - narrowAccountDetailBounds.y),
  ).toBeLessThan(1);
});

test("健康接口保持公开", async ({ request }) => {
  test.skip(!process.env.MEDIAHUB_E2E_REAL_BACKEND, "需要先启动实际后端");
  const live = await request.get("/health/live");
  const ready = await request.get("/health/ready");
  expect(live.status()).toBe(200);
  expect(await live.json()).toEqual({ status: "ok" });
  expect(ready.status()).toBe(200);
  expect(await ready.json()).toEqual({ status: "ok" });
});
