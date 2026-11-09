import { expect, test } from "@playwright/test";

test("设置：修改管理员、登录期限与扫码状态", async ({ page }, testInfo) => {
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
  let qrStatus = "waiting";
  let qrStarts = 0;
  await page.route("**/api/v1/settings/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (
      path.endsWith("/security") ||
      path.endsWith("/credentials") ||
      path.endsWith("/session")
    )
      return route.fulfill({
        json: { username: "admin", session_lifetime_days: 7 },
      });
    if (path.endsWith("/115"))
      return route.fulfill({
        json: { connected: false, account_name: null, connected_at: null },
      });
    if (path.endsWith("/qr")) qrStarts++;
    if (path.endsWith("/qr") || path.endsWith("/poll"))
      return route.fulfill({
        json: {
          id: "qr1",
          qr_url: "data:image/png;base64,iVBORw0KGgo=",
          status: qrStatus,
          expires_at: "2099-01-01T00:00:00Z",
        },
      });
    return route.fulfill({ status: 204 });
  });
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "管理员账号" })).toBeVisible();
  await page.screenshot({
    path: `/tmp/settings-${testInfo.project.name}.png`,
    fullPage: true,
  });
  await page.getByLabel("当前密码", { exact: true }).fill("password123");
  await page.getByRole("button", { name: "保存修改" }).click();
  await expect(
    page.getByText("管理员账号已更新，其他会话已退出。"),
  ).toBeVisible();
  await page.getByRole("button", { name: "115 账号" }).click();
  await page.getByRole("button", { name: "扫码登录" }).click();
  await expect(page.getByText("等待扫码")).toBeVisible();
  qrStatus = "expired";
  const refreshQr = page.getByRole("button", { name: "重新获取二维码" });
  await expect(refreshQr).toBeVisible();
  qrStatus = "waiting";
  await refreshQr.click();
  expect(qrStarts).toBe(2);
  await page.getByRole("button", { name: "取消扫码" }).click();
  await expect(page.getByText("尚未配置 115 账号，请扫码登录。")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
