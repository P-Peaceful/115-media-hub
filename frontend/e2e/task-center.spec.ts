import { expect, test } from "@playwright/test";

import type { components } from "../src/lib/api/generated/schema";

const now = "2026-11-20T00:00:00Z";
const taskTypes = [
  "scan",
  "strm",
  "sync",
  "check",
  "cleanup",
  "scrape",
  "upload",
] as const;
const library: components["schemas"]["Library"] = {
  id: "1",
  revision: 1,
  created_at: now,
  updated_at: now,
  name: "电视剧",
  directory_id: "0",
  directory_path: "115 网盘/电视剧",
  media_type: "tv",
  enabled: true,
  output_path: "/app/media/电视",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_poster_cover: true,
  language_preference: "zh-CN",
  scrape_source: "none",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: true,
  file_encoding: "UTF-8",
  strm_direct_domain: "",
  strm_link_format: "original_name",
  generate_nfo: true,
  link_type: "direct",
};
const completed: components["schemas"]["Task"] = {
  id: "task-1",
  type: "strm",
  library_id: library.id,
  library_name: library.name,
  source: "manual",
  schedule_id: null,
  parent_id: null,
  status: "completed",
  outcome: "partial_failure",
  created_at: now,
  started_at: now,
  finished_at: now,
  duration_ms: 73000,
  progress_percent: 100,
  speed_bytes_per_second: 8000000,
  current_file: null,
  total_items: 8,
  succeeded_items: 7,
  failed_items: 1,
  skipped_items: 0,
  actions: ["retry", "retry_failed"],
};

test.beforeEach(async ({ page }) => {
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
    route.fulfill({ json: { items: [library], next_cursor: null } }),
  );
  await page.route("**/api/v1/events", (route) =>
    route.fulfill({ status: 204 }),
  );
});

test("未注册执行器时，任务中心、筛选和计划编辑均可浏览", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({
      json: {
        items: taskTypes.map((type) => ({
          type,
          available: false,
          reason: "业务模块暂未接入",
          can_pause: false,
          can_retry_failed: false,
        })),
      },
    }),
  );
  await page.route("**/api/v1/tasks?**", (route) =>
    route.fulfill({
      json: {
        items: [],
        next_cursor: null,
        total: 0,
        by_type: {},
        by_status: {},
      },
    }),
  );
  await page.route("**/api/v1/schedules?**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: null } }),
  );
  await page.goto("/tasks");
  const screen = page.getByRole("main");
  await expect(
    page
      .getByRole("complementary", { name: "主导航" })
      .getByRole("link", { name: "任务中心", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(screen.getByText("暂无任务", { exact: true })).toBeVisible();
  const categories = screen.getByRole("radiogroup", { name: "任务分类" });
  await expect(categories.getByRole("radio")).toHaveCount(8);
  await page.screenshot({
    path: testInfo.outputPath("task-overview.png"),
    fullPage: true,
  });
  const status = screen
    .getByRole("radiogroup", { name: "状态分组" })
    .getByRole("radio", { name: /已暂停/ });
  await status.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/status=paused/);
  await expect(status).toBeChecked();
  await expect(screen.getByText("没有符合条件的任务")).toBeVisible();
  await page.goBack();
  await expect(screen.getByText("暂无任务", { exact: true })).toBeVisible();
  await screen.getByRole("link", { name: "定时计划" }).click();
  await expect(screen.getByText("暂无定时计划")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("schedule-list.png"),
    fullPage: true,
  });
  await screen.getByRole("link", { name: "创建计划" }).click();
  await expect(screen.getByRole("heading", { name: "创建计划" })).toBeVisible();
  await screen.getByRole("button", { name: "执行类型" }).click();
  await page.getByRole("option", { name: "STRM 生成（暂不可用）" }).click();
  await expect(screen.getByText("业务模块暂未接入")).toBeVisible();
  await expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  await expect(screen.getByText("执行参数")).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("schedule-editor.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
});

test("任务详情显示进度、失败原因和日志，并创建关联的失败重试任务", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/tasks/task-1", (route) =>
    route.fulfill({ json: completed }),
  );
  await page.route("**/api/v1/tasks/task-1/failures?**", (route) =>
    route.fulfill({
      json: {
        items: [
          { id: "f1", path: "/电视/S01E01.mkv", reason: "目标目录不可用" },
        ],
        next_cursor: null,
      },
    }),
  );
  await page.route("**/api/v1/tasks/task-1/logs?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "1",
            level: "error",
            message: "开始失败重试前检查目录",
            created_at: now,
          },
        ],
        next_cursor: null,
      },
    }),
  );
  const retry: components["schemas"]["Task"] = {
    ...completed,
    id: "task-2",
    source: "retry",
    parent_id: completed.id,
    status: "queued",
    outcome: null,
    started_at: null,
    finished_at: null,
    duration_ms: null,
    progress_percent: null,
    speed_bytes_per_second: null,
    total_items: null,
    succeeded_items: 0,
    failed_items: 0,
    actions: ["cancel"],
  };
  await page.route("**/api/v1/tasks/task-1/actions", async (route) => {
    expect(route.request().postDataJSON()).toEqual({ action: "retry_failed" });
    expect(route.request().headers()["x-csrf-token"]).toBe("token");
    await route.fulfill({ status: 202, json: retry });
  });
  await page.route("**/api/v1/tasks/task-2", (route) =>
    route.fulfill({ json: retry }),
  );
  await page.route("**/api/v1/tasks/task-2/failures?**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: null } }),
  );
  await page.goto("/tasks/task-1");
  await expect(page.getByText("已完成（部分失败）")).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveAttribute(
    "aria-valuenow",
    "100",
  );
  await expect(page.getByText("目标目录不可用")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("task-detail.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "查看日志" }).click();
  await expect(page.getByText("开始失败重试前检查目录")).toBeVisible();
  await expect(page.getByRole("tab", { name: "关联日志" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.getByRole("button", { name: "失败重试", exact: true }).click();
  await page.getByRole("menuitem", { name: "仅重试失败项" }).click();
  await expect(page).toHaveURL(/\/tasks\/task-2$/);
  await expect(page.getByRole("link", { name: "查看原任务" })).toHaveAttribute(
    "href",
    "/tasks/task-1",
  );
  await expect(page.getByRole("main").getByRole("checkbox")).toHaveCount(0);
});

test("保存和编辑每周计划时只提交调度和媒体库配置", async ({ page }) => {
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({
      json: {
        items: taskTypes.map((type) => ({
          type,
          available: type === "strm",
          reason: null,
          can_pause: false,
          can_retry_failed: false,
        })),
      },
    }),
  );
  const input = {
    name: "每周电视生成",
    type: "strm",
    library_id: "1",
    enabled: true,
    period: "weekly",
    timezone: "Asia/Shanghai",
    time: "02:00",
    weekdays: [0],
    notify_on: ["success", "failure", "partial_failure"],
  } satisfies components["schemas"]["ScheduleInput"];
  let schedule: components["schemas"]["Schedule"] | null = null;
  await page.route("**/api/v1/schedules?**", (route) =>
    route.fulfill({
      json: { items: schedule ? [schedule] : [], next_cursor: null },
    }),
  );
  await page.route("**/api/v1/schedules", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().postDataJSON()).toEqual(input);
    schedule = {
      ...input,
      id: "schedule-1",
      revision: 1,
      created_at: now,
      updated_at: now,
      next_run_at: "2026-11-21T18:00:00Z",
    };
    await route.fulfill({ status: 201, json: schedule });
  });
  await page.route("**/api/v1/schedules/schedule-1", async (route) => {
    if (route.request().method() === "PUT") {
      expect(route.request().headers()["x-schedule-revision"]).toBe("1");
      expect(route.request().postDataJSON()).toEqual({
        ...input,
        name: "每周电视生成更新",
      });
      if (!schedule) throw new Error("计划尚未保存");
      schedule = { ...schedule, name: "每周电视生成更新", revision: 2 };
    }
    await route.fulfill({ json: schedule });
  });
  await page.goto("/tasks/schedules/new");
  await page.getByRole("textbox", { name: "计划名称" }).fill(input.name);
  await page.getByRole("button", { name: "执行类型" }).click();
  await page.getByRole("option", { name: "STRM 生成", exact: true }).click();
  await page.getByRole("button", { name: "目标媒体库" }).click();
  await page.getByRole("option", { name: "电视剧" }).click();
  await page.getByRole("radio", { name: "每天" }).press("ArrowRight");
  await expect(page.getByRole("radio", { name: "每周" })).toBeChecked();
  await page.getByRole("checkbox", { name: "周一" }).press("Space");
  await page.getByRole("checkbox", { name: "周日" }).press("Space");
  const timeField = page.getByRole("group", { name: "执行时间" });
  const hours = timeField.getByRole("spinbutton").first();
  await hours.press("ArrowDown");
  await page
    .getByRole("switch", { name: "任务完成后发送站内通知" })
    .press("Space");
  await page.getByRole("button", { name: "保存" }).click();
  await expect(page).toHaveURL(/\/tasks\/schedules$/);
  await expect(page.getByText("2026/11/22 02:00:00")).toBeVisible();
  await page.getByRole("link", { name: input.name }).click();
  await expect(page.getByRole("checkbox", { name: "周日" })).toBeChecked();
  await page
    .getByRole("textbox", { name: "计划名称" })
    .fill("每周电视生成更新");
  await page.getByRole("button", { name: "保存" }).click();
  await expect(
    page.getByRole("link", { name: "每周电视生成更新" }),
  ).toBeVisible();
});

test("已连接的页面收到任务更新和站内通知", async ({ page }) => {
  let sendEvent = () => {};
  const eventReady = new Promise<void>((resolve) => {
    sendEvent = resolve;
  });
  let notificationDelivered = false;
  await page.route("**/api/v1/events", async (route) => {
    if (notificationDelivered) return route.fulfill({ status: 204 });
    await eventReady;
    notificationDelivered = true;
    return route.fulfill({
      contentType: "text/event-stream",
      body: 'event: task.changed\ndata: {"id":"task-1","notification":"partial_failure"}\n\n',
    });
  });
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/api/v1/tasks?**", (route) =>
    route.fulfill({
      json: {
        items: notificationDelivered ? [completed] : [],
        next_cursor: null,
        total: notificationDelivered ? 1 : 0,
        by_type: notificationDelivered ? { strm: 1 } : {},
        by_status: notificationDelivered ? { completed: 1 } : {},
      },
    }),
  );
  await page.goto("/tasks");
  const screen = page.getByRole("main");
  await expect(screen.getByText("暂无任务", { exact: true })).toBeVisible();
  sendEvent();
  await expect(
    screen.getByRole("link", { name: "STRM 生成 · 电视剧" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "查看任务", exact: true }),
  ).toHaveAttribute("href", "/tasks/task-1");
});

test("计划类型下拉菜单跟随应用主题切换", async ({ page }) => {
  await page.route("**/api/v1/task-types", (route) =>
    route.fulfill({
      json: {
        items: taskTypes.map((type) => ({
          type,
          available: false,
          reason: "业务模块暂未接入",
          can_pause: false,
          can_retry_failed: false,
        })),
      },
    }),
  );
  await page.goto("/tasks/schedules/new");

  const trigger = page.getByRole("button", { name: "执行类型" });
  const listbox = page.getByRole("listbox");
  const theme = page.locator("html");
  await expect(theme).toHaveAttribute("data-theme", /^(light|dark)$/);
  const field = page.getByRole("textbox", { name: "计划名称" });
  await expect(field).toBeVisible();
  const fillColors = await field.evaluate((item) => {
    const region = item.closest("section");
    const card = region?.firstElementChild;
    if (!card) throw new Error("Missing execution type card");
    return {
      field: getComputedStyle(item).backgroundColor,
      card: getComputedStyle(card).backgroundColor,
    };
  });
  expect(fillColors.field).not.toBe(fillColors.card);
  const wasDark = (await theme.getAttribute("data-theme")) === "dark";
  await trigger.click();
  await expect(listbox).toBeVisible();
  const previousBackground = await listbox.evaluate((item) =>
    getComputedStyle(item).getPropertyValue("--overlay"),
  );
  await page.keyboard.press("Escape");

  await page
    .getByRole("button", {
      name: wasDark ? "切换到亮色主题" : "切换到暗色主题",
    })
    .click();
  await expect(theme).toHaveAttribute("data-theme", wasDark ? "light" : "dark");
  await trigger.click();
  await expect(listbox).toBeVisible();
  const resolvedTheme = await listbox.evaluate((item) => ({
    overlay: getComputedStyle(item).getPropertyValue("--overlay"),
    root: getComputedStyle(document.documentElement).getPropertyValue(
      "--overlay",
    ),
  }));
  expect(resolvedTheme.overlay).toBe(resolvedTheme.root);
  expect(resolvedTheme.overlay).not.toBe(previousBackground);
});
