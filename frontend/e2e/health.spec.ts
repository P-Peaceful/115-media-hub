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

test("健康接口保持公开", async ({ request }) => {
  test.skip(!process.env.MEDIAHUB_E2E_REAL_BACKEND, "需要先启动实际后端");
  const live = await request.get("/health/live");
  const ready = await request.get("/health/ready");
  expect(live.status()).toBe(200);
  expect(await live.json()).toEqual({ status: "ok" });
  expect(ready.status()).toBe(200);
  expect(await ready.json()).toEqual({ status: "ok" });
});
