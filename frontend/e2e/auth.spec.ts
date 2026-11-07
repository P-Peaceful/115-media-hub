import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

import type { components } from "../src/lib/api/generated/schema";

const password = "Mediahub-test-123";
const session: components["schemas"]["AuthSession"] = {
  administrator: { id: "browser-admin", username: "admin" },
  expires_at: "2099-01-01T00:00:00Z",
  csrf_token: "browser-session-csrf",
};

async function mockAuthentication(page: Page, initialized = false) {
  await page.route("**/health/ready", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ status: "ok" }),
    });
  });
  let authenticated = false;
  await page.route("**/api/v1/auth/**", async (route) => {
    const action = new URL(route.request().url()).pathname.split("/").at(-1);
    let status = 200;
    let body: unknown;
    if (action === "bootstrap") {
      body = {
        initialized,
        authenticated,
        administrator: authenticated ? session.administrator : null,
        expires_at: authenticated ? session.expires_at : null,
        csrf_token: authenticated ? session.csrf_token : "browser-preauth-csrf",
      };
    } else {
      expect(route.request().headers()["x-csrf-token"]).toBe(
        authenticated ? session.csrf_token : "browser-preauth-csrf",
      );
      if (action === "logout") {
        authenticated = false;
        await route.fulfill({ status: 204 });
        return;
      }
      const input: unknown = route.request().postDataJSON();
      if (typeof input !== "object" || input === null || !("password" in input))
        throw new Error("Missing login input");
      if (input.password !== password) {
        status = 401;
        body = {
          error: {
            code: "invalid_credentials",
            message: "Invalid credentials",
            request_id: "browser-login",
          },
        };
      } else {
        initialized = true;
        authenticated = true;
        status = action === "initialize" ? 201 : 200;
        body = session;
      }
    }
    await route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

async function expectNoOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
}

test("初始化、退出、错误登录、记住登录与完整会话流程", async ({
  page,
  context,
}) => {
  const realBackend = !!process.env.MEDIAHUB_E2E_REAL_BACKEND;
  if (!realBackend) await mockAuthentication(page);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "首次使用 · 系统初始化" }),
  ).toBeVisible();
  await expect(page.getByLabel("输出目录")).toHaveValue("/app/media");
  await expect(page.getByLabel("服务地址")).toHaveValue(
    "http://127.0.0.1:5174",
  );
  await expect(page.getByText("115 授权入口")).toHaveCount(0);
  await expect(page.locator("header")).toHaveCount(0);
  await expectNoOverflow(page);
  await page.getByLabel("管理员账号").fill("admin");
  const overlongPassword = "a".repeat(33);
  await page.getByLabel("设置密码", { exact: true }).fill(overlongPassword);
  await page.getByLabel("确认密码", { exact: true }).fill(overlongPassword);
  await page.getByRole("button", { name: "开始初始化" }).click();
  await expect(page.getByLabel("设置密码", { exact: true })).toHaveValue(
    overlongPassword,
  );
  await expect(
    page.getByText("密码需为 8–32 位英文字母、数字或常用符号", { exact: true }),
  ).toHaveCount(2);
  await page.getByLabel("设置密码", { exact: true }).fill(password);
  await page.getByLabel("确认密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "显示设置密码" }).click();
  await expect(page.getByLabel("设置密码", { exact: true })).toHaveAttribute(
    "type",
    "text",
  );
  await page.getByRole("button", { name: "隐藏设置密码" }).click();
  const validity = page.getByRole("button", { name: /登录有效期/ });
  await validity.click();
  await page.getByRole("option", { name: "7 天" }).click();
  await page.getByRole("button", { name: "开始初始化" }).click();
  await expect(
    page.getByRole("heading", { name: "欢迎使用媒体中心" }),
  ).toBeVisible();
  await expect(page.getByRole("status")).toContainText(
    "后端服务与数据库连接正常",
  );
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "欢迎使用媒体中心" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "管理员登录" })).toBeVisible();
  await page.getByRole("button", { name: "忘记密码？" }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "mediahub admin reset-password",
  );
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "忘记密码？" })).toBeFocused();
  await page.getByLabel("用户名", { exact: true }).fill("admin");
  await page.getByLabel("密码", { exact: true }).fill("wrong-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("用户名或密码错误");
  const remember = page.getByRole("checkbox", { name: "记住登录状态" });
  await remember.focus();
  await page.keyboard.press("Space");
  await expect(remember).not.toBeChecked();
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByLabel("密码", { exact: true }).press("Enter");
  await expect(
    page.getByRole("heading", { name: "欢迎使用媒体中心" }),
  ).toBeVisible();
  if (realBackend) {
    const cookie = (await context.cookies()).find(
      (cookie) => cookie.name === "mediahub_session",
    );
    expect(cookie).toMatchObject({
      httpOnly: true,
      sameSite: "Lax",
      expires: -1,
    });
  }
  expect(
    await page.evaluate(() => ({
      local: Object.keys(localStorage),
      session: Object.keys(sessionStorage),
    })),
  ).toEqual({ local: [], session: [] });
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "管理员登录" })).toBeVisible();
  await expectNoOverflow(page);
});

test("安全的本地返回地址与远程目标阻断", async ({ page }) => {
  await mockAuthentication(page, true);
  await page.goto("/login?return_to=https%3A%2F%2Fevil.example");
  await page.getByLabel("用户名", { exact: true }).fill("admin");
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL("http://127.0.0.1:5174/");
  await page.getByRole("button", { name: "退出登录" }).click();
  await page.goto("/library?tab=recent#media");
  await expect(page).toHaveURL(/login\?return_to=/);
  await page.getByLabel("用户名", { exact: true }).fill("admin");
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(
    "http://127.0.0.1:5174/library?tab=recent#media",
  );
  await expect(page.getByRole("heading", { name: "页面不存在" })).toBeVisible();
});
