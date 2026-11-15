import { expect, test } from "@playwright/test";

const longName = "Library".repeat(14);
const longPath = `/app/media/${"long-segment".repeat(45)}`;

test("媒体库长文本在窄屏保持可读，组件保留 HeroUI 样式", async ({
  page,
}, testInfo) => {
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
      json: {
        items: [1, 2].map((id) => ({
          id: String(id),
          revision: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
          name: id === 1 ? longName : "电视剧库",
          description: "",
          directory_id: "0",
          directory_path: longPath,
          media_type: "movie",
          enabled: true,
          output_path: longPath,
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
        })),
        next_cursor: null,
      },
    }),
  );
  if (testInfo.project.name === "mobile-dark")
    await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/libraries");
  const libraryLink = page.getByRole("link", { name: longName });
  await expect(libraryLink).toBeVisible();
  const first = await libraryLink
    .locator('xpath=ancestor::*[@data-slot="card"]')
    .boundingBox();
  const second = await page
    .getByRole("link", { name: "电视剧库" })
    .locator('xpath=ancestor::*[@data-slot="card"]')
    .boundingBox();
  expect(first).not.toBeNull();
  expect(second).not.toBeNull();
  if (first && second) {
    if (testInfo.project.name === "desktop") {
      expect(first.y).toBe(second.y);
      expect(second.x).toBeGreaterThan(first.x);
    } else {
      expect(second.y).toBeGreaterThan(first.y);
    }
  }
  const overflow = await page
    .locator("main")
    .evaluate((element) => element.scrollWidth > element.clientWidth);
  expect(overflow).toBe(false);
  await expect(libraryLink).toHaveCSS("overflow-wrap", "anywhere");
  await expect(libraryLink).toHaveCSS("text-decoration-line", "none");
  await libraryLink.focus();
  await expect(libraryLink).not.toHaveCSS("outline-width", "3px");
  await expect(page.locator(".media-library-shell")).toHaveCount(1);
});

test("不同类型卡片展示真实配置、状态及不可用统计", async ({
  page,
}, testInfo) => {
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
  const types = ["movie", "tv", "anime", "documentary", "mixed"] as const;
  const names = ["电影库", "电视剧库", "动漫库", "纪录片库", "混合库"];
  const icons = ["film", "monitor", "panda", "file-text", "layers"];
  await page.route("**/api/v1/libraries?**", (route) =>
    route.fulfill({
      json: {
        items: types.map((type, index) => ({
          id: String(index + 1),
          revision: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
          name: names[index],
          description: "",
          directory_id: "0",
          directory_path: `115 网盘/${names[index]}`,
          media_type: type,
          enabled: index !== 3,
          output_path: `/media/${index}`,
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
        })),
        next_cursor: null,
      },
    }),
  );
  await page.setViewportSize({
    width: testInfo.project.name === "desktop" ? 1448 : 390,
    height: 900,
  });
  await page.goto("/libraries");
  for (const [index, name] of names.entries()) {
    const card = page
      .getByRole("link", { name, exact: true })
      .locator('xpath=ancestor::*[@data-slot="card"]');
    await expect(card.locator(`svg.lucide-${icons[index]}`)).toBeVisible();
    await expect(
      card.getByText(
        types[index] === "tv"
          ? "电视剧"
          : ["电影", "电视剧", "动漫", "纪录片", "混合"][index]!,
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      card
        .getByText(index === 3 ? "已暂停" : "已启用", { exact: true })
        .first(),
    ).toBeVisible();
    await expect(card.getByText(`115 网盘/${name}`)).toBeVisible();
    await expect(card.getByText(`/media/${index}`)).toBeVisible();
    await expect(card.getByText("暂无数据")).toHaveCount(2);
    await expect(card.getByText("运行中")).toHaveCount(0);
    await expect(card.getByRole("button", { name: "编辑" })).toBeVisible();
    await expect(card.getByRole("button", { name: "删除" })).toBeVisible();
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page
    .locator("main")
    .screenshot({ path: testInfo.outputPath("library-cards.png") });
});
