import { expect, test } from "@playwright/test";

test("媒体类型可辨认并支持键盘选择", async ({ page }, testInfo) => {
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
  await page.goto("/libraries/new");
  const group = page.getByRole("radiogroup", { name: "媒体类型" });
  const movie = group.getByRole("radio", { name: "电影" });
  const tv = group.getByRole("radio", { name: "电视剧" });
  await expect(movie).toBeChecked();
  await expect(tv).not.toBeChecked();
  const control = group.locator('[data-slot="radio-control"]').nth(1);
  await expect(control).toBeVisible();
  await expect(control).toHaveClass(/radio/);
  await movie.focus();
  await expect(movie).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(tv).toBeFocused();
  await expect(tv).toBeChecked();
  await expect(movie).not.toBeChecked();
  await group.screenshot({ path: testInfo.outputPath("media-type.png") });
});
