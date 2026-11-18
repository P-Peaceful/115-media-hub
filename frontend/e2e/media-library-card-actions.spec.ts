import { expect, test } from "@playwright/test";

const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "电影库",
  directory_id: "0",
  directory_path: "115 网盘/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/app/media/电影",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_poster_cover: true,
  language_preference: "zh-CN",
  scrape_source: "tmdb",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: true,
  file_encoding: "UTF-8",
  strm_direct_domain: "",
  strm_link_format: "original_name",
  generate_nfo: true,
  link_type: "direct",
};
for (const theme of ["light", "dark"] as const) {
  test(`媒体库卡片操作和状态（${theme}）`, async ({ page }, testInfo) => {
    const browserWarnings: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "error" || message.type() === "warning")
        browserWarnings.push(message.text());
    });
    await page.emulateMedia({ colorScheme: theme });
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
    let enabled = true;
    await page.route("**/api/v1/libraries/1/enabled", async (route) => {
      const data = route.request().postDataJSON() as { enabled: boolean };
      enabled = data.enabled;
      await route.fulfill({ json: { ...library, enabled } });
    });
    await page.route("**/api/v1/libraries?**", (route) =>
      route.fulfill({
        json: { items: [{ ...library, enabled }], next_cursor: null },
      }),
    );
    await page.goto("/libraries");
    const card = page.locator('main [data-slot="card"]').first();
    const edit = card.getByRole("button", { name: "编辑" });
    const remove = card.getByRole("button", { name: "删除" });
    const toggle = card.getByRole("switch", { name: "电影库启用状态" });
    await expect(toggle).toBeChecked();
    for (const property of [
      "background-color",
      "color",
      "font-size",
      "border-radius",
    ] as const) {
      const editStyle = await edit.evaluate(
        (element, prop) => getComputedStyle(element).getPropertyValue(prop),
        property,
      );
      await expect(remove).toHaveCSS(property, editStyle);
    }
    const cardBox = await card.boundingBox();
    const control = card.locator('[data-slot="switch-control"]');
    const switchBox = await control.boundingBox();
    const editBox = await edit.boundingBox();
    expect(cardBox && switchBox && editBox).toBeTruthy();
    if (!cardBox || !switchBox || !editBox) throw new Error("卡片操作不可见");
    expect(switchBox.x).toBeGreaterThan(editBox.x);
    expect(switchBox.y).toBeGreaterThan(cardBox.y + cardBox.height / 2);
    expect(
      cardBox.x + cardBox.width - (switchBox.x + switchBox.width),
    ).toBeLessThan(36);
    await card.screenshot({ path: testInfo.outputPath(`card-${theme}.png`) });
    await control.click();
    await expect(toggle).not.toBeChecked();
    await page.reload();
    await expect(
      card.getByRole("switch", { name: "电影库启用状态" }),
    ).not.toBeChecked();
    await toggle.focus();
    await page.keyboard.press("Space");
    await expect(toggle).toBeChecked();
    await card.getByRole("button", { name: "删除" }).click();
    const dialog = page.getByRole("alertdialog", { name: "删除媒体库？" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "取消" }).click();
    await expect(remove).toBeFocused();
    await edit.focus();
    await expect(edit).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/libraries\/1\/edit$/);
    expect(
      browserWarnings.filter(
        (message) =>
          message.includes("custom `render`") ||
          message.includes("Ref was not connected"),
      ),
    ).toEqual([]);
  });
}

test("删除警示弹窗处理长名称、待处理与失败重试", async ({ page }, testInfo) => {
  const name = "家庭电影与珍藏纪录片媒体库".repeat(5);
  let requests = 0;
  let deleted = false;
  let finishFirst: (() => void) | undefined;
  const firstResponse = new Promise<void>((resolve) => {
    finishFirst = resolve;
  });
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
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({
      json: { items: deleted ? [] : [{ ...library, name }], next_cursor: null },
    }),
  );
  await page.route("**/api/v1/libraries/1", async (route) => {
    expect(route.request().method()).toBe("DELETE");
    requests += 1;
    if (requests === 1) {
      await firstResponse;
      await route.fulfill({
        status: 503,
        json: {
          error: {
            code: "service_unavailable",
            message: "error",
            request_id: "test",
          },
        },
      });
      return;
    }
    deleted = true;
    await route.fulfill({ status: 204 });
  });
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 1000,
  });
  await page.goto("/libraries");
  const remove = page.getByRole("button", { name: "删除", exact: true });
  await remove.click();
  const dialog = page.getByRole("alertdialog", { name: "删除媒体库？" });
  const cancel = dialog.getByRole("button", { name: "取消", exact: true });
  await expect(cancel).toBeFocused();
  await expect(dialog).toHaveAccessibleDescription(
    /仅删除媒体库配置，网盘及本地文件不会被删除/,
  );
  await expect(dialog.locator('[data-slot="alert-dialog-icon"]')).toBeVisible();
  expect(
    await dialog.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  await expect(
    page.locator('[data-slot="alert-dialog-container"]'),
  ).not.toHaveAttribute("data-entering", "true");
  const box = await dialog.boundingBox();
  if (!box) throw new Error("删除警示弹窗未显示");
  expect(box.width).toBeLessThanOrEqual(384);
  await page.screenshot({
    path: testInfo.outputPath("delete-alert-dialog.png"),
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(remove).toBeFocused();
  expect(requests).toBe(0);
  await remove.click();
  await dialog.getByRole("button", { name: "确认删除" }).click();
  await expect(dialog.getByRole("status")).toHaveText("正在删除配置，请稍候…");
  await expect(cancel).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "正在删除…" }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await page.mouse.click(3, 3);
  await expect(dialog).toBeVisible();
  expect(requests).toBe(1);
  finishFirst?.();
  await expect(dialog.getByRole("alert")).toContainText(
    "服务暂时不可用，请稍后重试。",
  );
  await expect(cancel).toBeEnabled();
  await page.screenshot({
    path: testInfo.outputPath("delete-alert-error.png"),
    fullPage: true,
  });
  await dialog.getByRole("button", { name: "确认删除" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByText("创建媒体库，开始管理您的媒体配置。", { exact: true }),
  ).toBeVisible();
  expect(requests).toBe(2);
});
