import { expect, test } from "@playwright/test";

test("115 真实信息、失效和重新授权入口", async ({ page }, testInfo) => {
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
  await page.route("**/api/v1/system/status", (route) =>
    route.fulfill({ json: { uptime_seconds: 62, version: "dev" } }),
  );
  await page.route("**/api/v1/settings/115", (route) =>
    route.fulfill({
      json: {
        connected: true,
        account_name: null,
        connected_at: "2026-01-01T00:00:00Z",
      },
    }),
  );
  let status = "valid";
  await page.route("**/api/v1/settings/115/details", (route) =>
    route.fulfill({
      json:
        status === "valid"
          ? {
              status,
              profile: {
                user_name: "示例测试用户",
                user_id: "123456789012345678",
                avatar:
                  "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=",
                is_member: true,
                member_expires_at: "2027-01-01T00:00:00Z",
                member_forever: false,
              },
              space: {
                used_bytes: "1099511627776",
                total_bytes: "2199023255552",
              },
              space_status: "available",
              devices: [
                {
                  name: "测试手机",
                  type: "iOS",
                  ip: "127.0.0.1",
                  city: "上海",
                  login_at: "2026-11-08T12:00:00Z",
                  is_current: true,
                },
              ],
              devices_status: "available",
            }
          : {
              status,
              profile: null,
              space: null,
              space_status: "unavailable",
              devices: [],
              devices_status: "unavailable",
            },
    }),
  );
  if (testInfo.project.name === "desktop")
    await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/");
  if (testInfo.project.name === "desktop")
    await expect(
      page
        .getByRole("complementary", { name: "主导航" })
        .getByText("账号：示例测试用户"),
    ).toBeVisible();
  await expect(
    page.locator("main").getByText("账号：示例测试用户"),
  ).toBeVisible();
  await expect(page.getByText("1.00 TB", { exact: true })).toBeVisible();
  await page.screenshot({
    path: `/tmp/account-dashboard-${testInfo.project.name}.png`,
    fullPage: true,
  });
  await page
    .locator("main")
    .getByRole("link", { name: "管理授权", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "115 账号" })).toBeVisible();
  await expect(page.getByText("123456789012345678")).toBeVisible();
  await expect(
    page.getByRole("img", { name: "示例测试用户头像" }),
  ).toBeVisible();
  await expect(page.getByText("115 会员")).toBeVisible();
  await expect(page.getByText("会员有效期")).toBeVisible();
  await expect(page.getByText("测试手机")).toBeVisible();
  await expect(page.getByText("当前设备")).toBeVisible();
  await expect(
    page.getByText("1.00 TB / 2.00 TB（已用 / 总容量）"),
  ).toBeVisible();
  await page.screenshot({
    path: `/tmp/account-settings-${testInfo.project.name}.png`,
    fullPage: true,
  });
  status = "invalid";
  await page.getByRole("button", { name: "检查授权状态" }).click();
  await expect(page.getByText("授权已失效，请重新扫码授权。")).toBeVisible();
  await page.reload();
  await expect(page.getByText("授权已失效，请重新扫码授权。")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "重新扫码授权" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("扫码成功立即刷新共享账号信息", async ({ page }) => {
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
  let authorized = false;
  await page.route("**/api/v1/settings/115**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/details"))
      return route.fulfill({
        json: authorized
          ? {
              status: "valid",
              profile: {
                user_name: "新账号",
                user_id: null,
                avatar: null,
                is_member: null,
                member_expires_at: null,
                member_forever: null,
              },
              space: null,
              space_status: "unavailable",
              devices: [],
              devices_status: "unavailable",
            }
          : {
              status: "unconfigured",
              profile: null,
              space: null,
              space_status: "unavailable",
              devices: [],
              devices_status: "unavailable",
            },
      });
    if (path.endsWith("/115"))
      return route.fulfill({
        json: {
          connected: authorized,
          account_name: authorized ? "新账号" : null,
          connected_at: null,
        },
      });
    if (path.endsWith("/poll")) authorized = true;
    return route.fulfill({
      json: {
        id: "qr1",
        qr_url: "data:image/png;base64,iVBORw0KGgo=",
        status: authorized ? "succeeded" : "waiting",
        expires_at: "2099-01-01T00:00:00Z",
      },
    });
  });
  await page.goto("/settings?tab=account");
  await page.getByRole("button", { name: "扫码登录" }).click();
  await expect(page.getByRole("button", { name: "完成" })).toBeVisible();
  await page.getByRole("link", { name: "总览" }).click();
  await expect(page.locator("main").getByText("账号：新账号")).toBeVisible();
});
