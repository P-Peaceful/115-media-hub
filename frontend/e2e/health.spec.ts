import { expect, test } from "@playwright/test";

test("检查状态、键盘重试和窄屏布局", async ({ page }) => {
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
    "欢迎使用媒体中心",
  );
  await expect(page.getByRole("status")).toContainText("服务暂未就绪");
  ready = true;
  await page.keyboard.press("Tab");
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
});

test("通过开发代理连接实际后端", async ({ page }) => {
  test.skip(!process.env.MEDIAHUB_E2E_REAL_BACKEND, "需要先启动实际后端");
  await page.goto("/");
  await expect(page.getByRole("status")).toContainText(
    "后端服务与数据库连接正常。",
  );
});
