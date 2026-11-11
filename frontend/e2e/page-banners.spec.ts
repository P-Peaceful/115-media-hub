import { expect, test } from "@playwright/test";

const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "长媒体库名称".repeat(15),
  description: "",
  directory_id: "0",
  directory_path: "115 网盘",
  media_type: "movie",
  enabled: true,
  output_path: "/app/media",
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
  test(`所有模块横幅遵循总览样式（${theme}）`, async ({ page }, testInfo) => {
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
    await page.route("**/api/v1/settings/security", (route) =>
      route.fulfill({ json: { username: "admin", session_lifetime_days: 7 } }),
    );
    await page.route("**/api/v1/settings/115", (route) =>
      route.fulfill({
        json: { connected: false, account_name: null, connected_at: null },
      }),
    );
    await page.route("**/api/v1/system/status", (route) =>
      route.fulfill({ json: { uptime_seconds: 60, version: "dev" } }),
    );
    await page.route("**/api/v1/cloud-drive/files?**", (route) =>
      route.fulfill({
        json: {
          directory_id: "0",
          path: [{ id: "0", name: "115 网盘" }],
          items: [
            {
              id: "2",
              name: "影片.mp4",
              is_directory: false,
              size_bytes: "1024",
              modified_at: null,
              file_type: "video",
            },
          ],
          next_cursor: null,
        },
      }),
    );
    await page.route("**/api/v1/libraries/1", (route) =>
      route.fulfill({ json: library }),
    );
    await page.route("**/api/v1/libraries?**", (route) =>
      route.fulfill({ json: { items: [library], next_cursor: null } }),
    );
    const pages = [
      ["overview", "/", "欢迎使用 115 Media Hub"],
      ["cloud", "/cloud-drive", "网盘浏览"],
      ["settings", "/settings", "设置"],
      ["libraries", "/libraries", "媒体库"],
      ["new", "/libraries/new", "新建媒体库"],
      ["detail", "/libraries/1", library.name],
      ["edit", "/libraries/1/edit", "编辑媒体库"],
    ] as const;
    let gradient = "";
    let baseline: {
      left: number;
      width: number;
      top: number;
      padding: string;
      fontSize: string;
      fontWeight: string;
      lineHeight: string;
      imageWidth: number;
      imageSource: string;
    } | null = null;
    for (const [name, url, title] of pages) {
      await page.goto(url);
      const banner = page.locator('main [data-slot="page-banner"]');
      await expect(banner.getByRole("heading", { level: 1 })).toContainText(
        title,
      );
      const style = await banner.evaluate(
        (element) => getComputedStyle(element).backgroundImage,
      );
      if (!gradient) gradient = style;
      expect(style).toBe(gradient);
      await expect(banner.getByRole("heading", { level: 1 })).toHaveCSS(
        "color",
        "rgb(17, 21, 42)",
      );
      await expect(banner.locator("p")).toHaveCSS("color", "rgb(85, 94, 122)");
      const metrics = await banner.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        const heading = element.querySelector("h1");
        const image = element.querySelector("img");
        if (!heading || !image) throw new Error("横幅内容缺失");
        const headingStyle = getComputedStyle(heading);
        return {
          left: bounds.left,
          width: bounds.width,
          top:
            bounds.top -
            (element.closest("main")?.getBoundingClientRect().top ?? 0),
          padding: style.padding,
          fontSize: headingStyle.fontSize,
          fontWeight: headingStyle.fontWeight,
          lineHeight: headingStyle.lineHeight,
          margin: headingStyle.margin,
          imageWidth: image.getBoundingClientRect().width,
          imageSource: image.src,
        };
      });
      if (!baseline) baseline = metrics;
      expect(metrics).toEqual(baseline);
      expect(metrics.fontWeight).toBe("650");
      expect(metrics.margin).toBe("0px 0px 7px");
      const expectedSize = Math.max(
        24,
        Math.min(34, (page.viewportSize()?.width ?? 0) * 0.024),
      );
      expect(Number.parseFloat(metrics.fontSize)).toBeCloseTo(expectedSize, 1);
      if (name === "cloud") {
        await page
          .getByRole("button", { name: "影片.mp4", exact: true })
          .click();
        await expect(
          page.getByRole("complementary", { name: "文件详情" }),
        ).toBeVisible();
        const expandedWidth = await banner.evaluate(
          (element) => element.getBoundingClientRect().width,
        );
        expect(expandedWidth).toBe(baseline.width);
      }
      const dimensions = await banner.evaluate((element) => {
        const box = element.getBoundingClientRect();
        const heading = element.querySelector("h1")?.getBoundingClientRect();
        const paragraph = element.querySelector("p")?.getBoundingClientRect();
        const artwork = element.querySelector("img")?.getBoundingClientRect();
        if (!heading || !paragraph || !artwork) throw new Error("横幅内容缺失");
        return {
          top: box.top,
          bottom: box.bottom,
          headingTop: heading.top,
          headingBottom: heading.bottom,
          headingRight: heading.right,
          imageLeft: artwork.left,
          height: box.height,
          paragraphBottom: paragraph.bottom,
          imageWidth: artwork.width,
        };
      });
      expect(dimensions.headingTop).toBeGreaterThanOrEqual(dimensions.top);
      expect(dimensions.headingBottom).toBeLessThanOrEqual(dimensions.bottom);
      expect(dimensions.paragraphBottom).toBeLessThanOrEqual(dimensions.bottom);
      expect(dimensions.headingRight).toBeLessThanOrEqual(dimensions.imageLeft);
      if (testInfo.project.name === "desktop" && name !== "detail")
        expect(dimensions.height).toBe(126);
      if (testInfo.project.name === "mobile-dark")
        expect(dimensions.imageWidth).toBe(90);
      await banner.screenshot({
        path: testInfo.outputPath(`${name}-${theme}.png`),
      });
    }
    await page.getByRole("link", { name: "返回媒体库" }).click();
    await expect(page).toHaveURL(/\/libraries$/);
  });
}
