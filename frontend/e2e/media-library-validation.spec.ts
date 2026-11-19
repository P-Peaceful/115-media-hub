import { expect, test } from "@playwright/test";

test("绑定目录必填提示在选择后清除，随后可以创建", async ({ page }) => {
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
  await page.route("**/api/v1/cloud-drive/files?**", (route) =>
    route.fulfill({
      json: {
        directory_id: "0",
        path: [{ id: "0", name: "115 网盘" }],
        items: [],
        next_cursor: null,
        total: 0,
        page: 1,
        page_size: 50,
        total_pages: 1,
      },
    }),
  );
  let creates = 0;
  let saved: Record<string, unknown> | undefined;
  let updates = 0;
  await page.route("**/api/v1/libraries/1", (route) => {
    if (!saved) throw new Error("媒体库尚未创建");
    if (route.request().method() === "PUT") {
      updates++;
      const payload: unknown = route.request().postDataJSON();
      if (typeof payload !== "object" || payload === null)
        throw new Error("无效的更新请求");
      return route.fulfill({
        json: { ...saved, ...payload },
      });
    }
    return route.fulfill({ json: saved });
  });
  await page.route("**/api/v1/libraries", (route) => {
    if (route.request().method() === "POST") {
      creates++;
      const payload: unknown = route.request().postDataJSON();
      if (typeof payload !== "object" || payload === null)
        throw new Error("无效的创建请求");
      saved = {
        id: "1",
        revision: 1,
        enabled: true,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        ...payload,
      };
      return route.fulfill({ status: 201, json: saved });
    }
    return route.fulfill({ json: { items: [], next_cursor: null } });
  });
  await page.goto("/libraries/new");
  const directory = page.getByRole("button", {
    name: "媒体库目录 *",
    exact: true,
  });
  await page.getByRole("button", { name: "创建媒体库" }).click();
  await expect(page.getByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  await expect(directory).toHaveAccessibleDescription(
    "请选择绑定的 115 网盘目录。",
  );
  await page.getByLabel("媒体库名称 *").fill("我的电影");
  await page.getByLabel("输出目录 *", { exact: true }).fill("/app/media/电影");
  await page.getByRole("button", { name: "创建媒体库" }).click();
  await expect(page.getByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  expect(creates).toBe(0);
  await directory.click();
  await page.getByRole("button", { name: "选择当前目录" }).click();
  await expect(page.getByText("请选择绑定的 115 网盘目录。")).toHaveCount(0);
  await expect(directory).toHaveText("115 网盘");
  await expect(directory).not.toHaveAttribute("aria-describedby");
  await page.getByRole("button", { name: "创建媒体库" }).click();
  await expect.poll(() => creates).toBe(1);
  await page.goto("/libraries/1/edit");
  await expect(directory).toHaveText("115 网盘");
  await page.getByRole("button", { name: "保存配置" }).click();
  await expect.poll(() => updates).toBe(1);
});
