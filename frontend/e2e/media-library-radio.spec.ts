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
  const contrast = await control.evaluate((element) => {
    const shadow = getComputedStyle(element).boxShadow;
    const ring = shadow.match(/(oklab\([^)]+\)) 0px 0px 0px 1px/)?.[1];
    const card = element.closest('[data-slot="card"]');
    if (!ring || !card) return 0;
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1;
    const context = canvas.getContext("2d");
    if (!context) return 0;
    context.fillStyle = getComputedStyle(card).backgroundColor;
    context.fillRect(0, 0, 1, 1);
    const background = context.getImageData(0, 0, 1, 1).data;
    context.fillStyle = ring;
    context.fillRect(0, 0, 1, 1);
    const border = context.getImageData(0, 0, 1, 1).data;
    const luminance = (pixel: Uint8ClampedArray) => {
      const linear = [pixel[0] ?? 0, pixel[1] ?? 0, pixel[2] ?? 0].map(
        (value) => {
          const channel = value / 255;
          return channel <= 0.04045
            ? channel / 12.92
            : ((channel + 0.055) / 1.055) ** 2.4;
        },
      );
      return (
        0.2126 * (linear[0] ?? 0) +
        0.7152 * (linear[1] ?? 0) +
        0.0722 * (linear[2] ?? 0)
      );
    };
    const first = luminance(background);
    const second = luminance(border);
    return (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05);
  });
  expect(contrast).toBeGreaterThanOrEqual(3);
  await movie.focus();
  await expect(movie).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(tv).toBeFocused();
  await expect(tv).toBeChecked();
  await expect(movie).not.toBeChecked();
  await group.screenshot({ path: testInfo.outputPath("media-type.png") });
});
