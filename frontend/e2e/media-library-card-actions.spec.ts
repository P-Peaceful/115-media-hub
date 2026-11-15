import { expect, test } from "@playwright/test";

const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "电影库",
  description: "",
  directory_id: "0",
  directory_path: "115 网盘/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/app/media/电影",
  generate_strm: true,
  scrape_metadata: true,
  generate_nfo: true,
  preserve_structure: true,
  structure_mode: "full",
  path_mappings: [],
  link_type: "direct",
  link_expiry: "default",
  filename_rule: "original",
  strm_extension: ".strm",
  nfo_name: "same_as_video",
  min_file_size_mb: 100,
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
    const dialog = page.getByRole("dialog", { name: "删除媒体库？" });
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
