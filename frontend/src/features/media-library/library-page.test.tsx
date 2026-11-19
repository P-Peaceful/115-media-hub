import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { server } from "@/test/server";

import { decodeLibrary } from "./api";
import {
  LibraryDetailLayout,
  LibraryDetailPage,
  LibraryFormPage,
  LibraryListPage,
} from "./library-page";

vi.mock("@/features/auth/auth-provider", () => ({
  useAuth: () => ({ data: { csrf_token: "token" } }),
}));
const library = {
  id: "1",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  name: "电影库",
  directory_id: "123",
  directory_path: "115 网盘/电影",
  media_type: "movie",
  enabled: true,
  output_path: "/app/media/电影",
  generate_strm: true,
  strm_existing_file_policy: "skip",
  generate_nfo: true,
  generate_poster_cover: true,
  language_preference: "zh-CN",
  scrape_source: "tmdb",
  exclude_directory_path: "",
  include_subdirectories: true,
  follow_directory_changes: true,
  file_encoding: "UTF-8",
  link_type: "proxy",
  strm_direct_domain: "",
  strm_link_format: "original_name",
};
function LocationProbe() {
  return <output aria-label="当前路径">{useLocation().pathname}</output>;
}
function renderPage() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false, gcTime: 0 } },
        })
      }
    >
      <MemoryRouter>
        <LibraryListPage />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
describe("媒体库管理", () => {
  it("筛选并暂停媒体库，删除前确认", async () => {
    let enabled = true;
    let deleted = false;
    server.use(
      http.get("/api/v1/libraries", () =>
        HttpResponse.json({
          items: deleted ? [] : [{ ...library, enabled }],
          next_cursor: null,
        }),
      ),
      http.patch("/api/v1/libraries/1/enabled", async ({ request }) => {
        expect(request.headers.get("X-CSRF-Token")).toBe("token");
        enabled = ((await request.json()) as { enabled: boolean }).enabled;
        return HttpResponse.json({ ...library, enabled });
      }),
      http.delete("/api/v1/libraries/1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByRole("link", { name: "电影库" })).toBeVisible();
    const toggle = screen.getByRole("switch", { name: "电影库启用状态" });
    expect(toggle).toBeChecked();
    await user.click(toggle);
    await vi.waitFor(() => expect(toggle).not.toBeChecked());
    const card = screen
      .getByRole("link", { name: "电影库" })
      .closest('[data-slot="card"]');
    expect(card).not.toBeNull();
    expect(within(card as HTMLElement).getAllByText("已暂停")).toHaveLength(2);
    await user.click(toggle);
    await vi.waitFor(() => expect(toggle).toBeChecked());
    await user.click(toggle);
    await vi.waitFor(() => expect(toggle).not.toBeChecked());
    await user.click(screen.getByRole("radio", { name: /已启用/ }));
    expect(screen.getByText("当前筛选条件下没有媒体库。")).toBeVisible();
    await user.click(screen.getByRole("radio", { name: /全部/ }));
    await user.click(screen.getByRole("button", { name: "删除" }));
    const dialog = screen.getByRole("alertdialog", { name: "删除媒体库？" });
    expect(within(dialog).getByText(/网盘及本地文件不会被删除/)).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "确认删除" }));
    expect(
      await screen.findByText("创建媒体库，开始管理您的媒体配置。"),
    ).toBeVisible();
  });
  it("编辑按钮进入配置页", async () => {
    server.use(
      http.get("/api/v1/libraries", () =>
        HttpResponse.json({ items: [library], next_cursor: null }),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    const edit = await screen.findByRole("button", { name: "编辑" });
    await user.click(edit);
    expect(screen.getByLabelText("当前路径")).toHaveTextContent(
      "/libraries/1/edit",
    );
  });
  it("更新失败时保留原状态并允许重试", async () => {
    let requests = 0;
    let enabled = true;
    server.use(
      http.get("/api/v1/libraries", () =>
        HttpResponse.json({
          items: [{ ...library, enabled }],
          next_cursor: null,
        }),
      ),
      http.patch("/api/v1/libraries/1/enabled", async () => {
        requests += 1;
        await delay(100);
        if (requests === 1)
          return HttpResponse.json(
            {
              error: {
                code: "service_unavailable",
                message: "error",
                request_id: "test",
              },
            },
            { status: 503 },
          );
        enabled = false;
        return HttpResponse.json({ ...library, enabled });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    const toggle = await screen.findByRole("switch", {
      name: "电影库启用状态",
    });
    await user.click(toggle);
    expect(toggle).toBeDisabled();
    expect(
      await screen.findByText("服务暂时不可用，请稍后重试。"),
    ).toBeVisible();
    expect(toggle).toBeEnabled();
    expect(toggle).toBeChecked();
    await user.click(toggle);
    await vi.waitFor(() => expect(toggle).not.toBeChecked());
    expect(requests).toBe(2);
  });
  it("列表失败时显示错误而非空列表", async () => {
    server.use(
      http.get("/api/v1/libraries", () =>
        HttpResponse.json(
          {
            error: {
              code: "service_unavailable",
              message: "error",
              request_id: "test",
            },
          },
          { status: 503 },
        ),
      ),
    );
    renderPage();
    expect(await screen.findByText("无法加载媒体库。")).toBeVisible();
    expect(screen.queryByText("暂无媒体库")).not.toBeInTheDocument();
  });
});

it("选择 115 目录并保存配置", async () => {
  let submitted: unknown;
  server.use(
    http.get("/api/v1/cloud-drive/files", () =>
      HttpResponse.json({
        directory_id: "0",
        total: 0,
        page: 1,
        page_size: 50,
        total_pages: 1,
        path: [{ id: "0", name: "115 网盘" }],
        items: [],
        next_cursor: null,
      }),
    ),
    http.post("/api/v1/libraries", async ({ request }) => {
      submitted = await request.json();
      return HttpResponse.json(library, { status: 201 });
    }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false, gcTime: 0 } },
        })
      }
    >
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.getByRole("heading", { name: "新建媒体库配置" })).toBeVisible();
  expect(screen.getByText(/目录绑定、输出与 STRM 等配置/)).toBeVisible();
  expect(screen.getByRole("button", { name: "取消" })).toBeVisible();
  expect(screen.getByRole("button", { name: "创建媒体库" })).toBeVisible();
  const outputOptions = screen.getByRole("group", { name: "生成内容" });
  expect(outputOptions).toHaveClass("sm:grid-cols-3");
  expect(within(outputOptions).getAllByRole("checkbox")).toHaveLength(3);
  await user.type(screen.getByLabelText("媒体库名称 *"), "我的电影");
  await user.type(screen.getByLabelText("输出目录 *"), "/app/media/电影");
  await user.click(screen.getByRole("button", { name: /媒体类型/ }));
  await user.click(await screen.findByRole("option", { name: "电视剧" }));
  const policy = screen.getByRole("button", { name: /STRM 已存在时/ });
  expect(policy).toHaveTextContent("跳过（默认）");
  await user.click(policy);
  await user.click(await screen.findByRole("option", { name: "覆盖" }));
  await user.click(screen.getByRole("checkbox", { name: "STRM 文件" }));
  expect(policy).toBeDisabled();
  expect(policy).toHaveTextContent("覆盖");
  await user.click(screen.getByRole("checkbox", { name: "STRM 文件" }));
  expect(policy).toBeEnabled();
  expect(policy).toHaveTextContent("覆盖");
  await user.click(screen.getByRole("checkbox", { name: "STRM 文件" }));
  await user.click(screen.getByRole("button", { name: /链接类型/ }));
  await screen.findByRole("option", { name: "302 代理" });
  await user.keyboard("{End}{Enter}");
  await user.click(screen.getByRole("button", { name: /媒体库目录/ }));
  await user.click(await screen.findByRole("button", { name: "选择当前目录" }));
  await user.click(screen.getByRole("button", { name: "选择排除目录" }));
  const excludeDialog = screen.getByRole("dialog", { name: "选择排除目录" });
  await user.click(
    await within(excludeDialog).findByRole("button", {
      name: "选择当前目录",
    }),
  );
  expect(screen.getByPlaceholderText("可选，输入网盘目录路径")).toHaveValue(
    "115 网盘",
  );
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  await vi.waitFor(() =>
    expect(submitted).toMatchObject({
      name: "我的电影",
      directory_id: "0",
      directory_path: "115 网盘",
      output_path: "/app/media/电影",
      media_type: "tv",
      generate_strm: false,
      strm_existing_file_policy: "overwrite",
      generate_poster_cover: true,
      exclude_directory_path: "115 网盘",
      link_type: "proxy",
      scrape_source: "tmdb",
      strm_link_format: "original_name",
    }),
  );
});

it("未选绑定目录时在字段处提示，选中后清除并允许提交", async () => {
  let submitted: unknown;
  server.use(
    http.get("/api/v1/cloud-drive/files", () =>
      HttpResponse.json({
        directory_id: "0",
        total: 0,
        page: 1,
        page_size: 50,
        total_pages: 1,
        path: [{ id: "0", name: "115 网盘" }],
        items: [],
        next_cursor: null,
      }),
    ),
    http.post("/api/v1/libraries", async ({ request }) => {
      submitted = await request.json();
      return HttpResponse.json(library, { status: 201 });
    }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const directory = screen.getByRole("button", { name: /媒体库目录/ });
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(await screen.findByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  expect(directory).toHaveAccessibleDescription("请选择绑定的 115 网盘目录。");
  expect(submitted).toBeUndefined();
  await user.type(screen.getByLabelText("媒体库名称 *"), "我的电影");
  await user.type(screen.getByLabelText("输出目录 *"), "/app/media/电影");
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(screen.getByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  expect(submitted).toBeUndefined();
  await user.click(directory);
  await user.click(await screen.findByRole("button", { name: "选择当前目录" }));
  expect(
    screen.queryByText("请选择绑定的 115 网盘目录。"),
  ).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  await vi.waitFor(() =>
    expect(submitted).toMatchObject({
      directory_id: "0",
      directory_path: "115 网盘",
    }),
  );
});

it("指出无效名称和相对输出路径", async () => {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await user.type(screen.getByLabelText("媒体库名称 *"), "   ");
  await user.type(screen.getByLabelText("输出目录 *"), "relative");
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(screen.getByLabelText("媒体库名称 *")).toHaveAccessibleDescription(
    "请输入媒体库名称。",
  );
  expect(screen.getByLabelText("媒体库名称 *")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByText("请输入媒体库名称。")).toBeVisible();
  expect(screen.getByLabelText("输出目录 *")).toHaveAccessibleDescription(
    "请输入以 / 开头的本地绝对路径。",
  );
  expect(screen.getByText("请输入以 / 开头的本地绝对路径。")).toBeVisible();
});

it("配置表单仅显示本期媒体库配置并提供 STRM 格式标签", async () => {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.getByLabelText("语言偏好")).toBeVisible();
  expect(screen.getByLabelText("刮削源")).toBeVisible();
  expect(screen.getByRole("tab", { name: "文件原名" })).toBeVisible();
  await user.click(screen.getByRole("tab", { name: "文件 ID" }));
  expect(screen.getByRole("tab", { name: "文件 ID" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  expect(screen.queryByText("文件规则")).not.toBeInTheDocument();
  expect(screen.queryByText("目录结构保留")).not.toBeInTheDocument();
  expect(screen.queryByText("链接有效期")).not.toBeInTheDocument();
  expect(screen.queryByText("自动清理")).not.toBeInTheDocument();
});

it("编辑保留读取时版本并显示保存冲突", async () => {
  let sentRevision: string | null = null;
  let submitted: unknown;
  server.use(
    http.get("/api/v1/libraries/1", () => HttpResponse.json(library)),
    http.put("/api/v1/libraries/1", async ({ request }) => {
      sentRevision = request.headers.get("X-Library-Revision");
      submitted = await request.json();
      return HttpResponse.json(
        {
          error: { code: "conflict", message: "conflict", request_id: "test" },
        },
        { status: 409 },
      );
    }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/edit"]}>
        <Routes>
          <Route path="/libraries/:id/edit" element={<LibraryFormPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const name = await screen.findByLabelText("媒体库名称 *");
  await user.clear(name);
  await user.type(name, "更新的电影库");
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  expect(
    await screen.findByText("媒体库已被修改，请刷新后重试。"),
  ).toBeVisible();
  expect(sentRevision).toBe("1");
  expect(submitted).toMatchObject({
    name: "更新的电影库",
    directory_id: "123",
    output_path: "/app/media/电影",
  });
});

it("编辑成功仅提交配置字段，版本通过请求头保留", async () => {
  let submitted: unknown;
  let sentRevision: string | null = null;
  server.use(
    http.get("/api/v1/libraries/1", () =>
      HttpResponse.json({ ...library, strm_existing_file_policy: "overwrite" }),
    ),
    http.put("/api/v1/libraries/1", async ({ request }) => {
      submitted = await request.json();
      sentRevision = request.headers.get("X-Library-Revision");
      return HttpResponse.json({
        ...library,
        name: "更新的电影库",
        revision: 2,
      });
    }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/edit"]}>
        <Routes>
          <Route path="/libraries/:id/edit" element={<LibraryFormPage />} />
          <Route path="/libraries" element={<p>配置已保存</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const name = await screen.findByLabelText("媒体库名称 *");
  const policy = screen.getByRole("button", { name: /STRM 已存在时/ });
  expect(policy).toHaveTextContent("覆盖");
  await user.click(policy);
  await user.click(await screen.findByRole("option", { name: "跳过（默认）" }));
  await user.clear(name);
  await user.type(name, "更新的电影库");
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  expect(await screen.findByText("配置已保存")).toBeVisible();
  expect(sentRevision).toBe("1");
  expect(submitted).toEqual({
    name: "更新的电影库",
    directory_id: "123",
    directory_path: "115 网盘/电影",
    media_type: "movie",
    output_path: "/app/media/电影",
    generate_strm: true,
    strm_existing_file_policy: "skip",
    generate_nfo: true,
    generate_poster_cover: true,
    language_preference: "zh-CN",
    scrape_source: "tmdb",
    exclude_directory_path: "",
    include_subdirectories: true,
    follow_directory_changes: true,
    file_encoding: "UTF-8",
    link_type: "proxy",
    strm_direct_domain: "",
    strm_link_format: "original_name",
  });
});

it("目录读取失败时提示重试", async () => {
  server.use(
    http.get("/api/v1/cloud-drive/files", () =>
      HttpResponse.json(
        {
          error: {
            code: "service_unavailable",
            message: "error",
            request_id: "test",
          },
        },
        { status: 503 },
      ),
    ),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: /媒体库目录/ }));
  expect(await screen.findByText(/无法读取目录/)).toBeVisible();
  expect(screen.getByRole("button", { name: "重试" })).toBeVisible();
});

it("详情展示真实配置和系统信息，标记演示数据并展示统一的操作按钮", async () => {
  server.use(
    http.get("/api/v1/libraries/1", () =>
      HttpResponse.json({
        ...library,
        scrape_source: "metatube",
        exclude_directory_path: "115 网盘/电影/样片",
        include_subdirectories: false,
        follow_directory_changes: true,
        generate_strm: false,
        generate_nfo: true,
        generate_poster_cover: false,
        strm_existing_file_policy: "overwrite",
        link_type: "direct",
        strm_direct_domain: "media.example.com",
        strm_link_format: "file_id",
      }),
    ),
    http.get("/api/v1/system/status", () =>
      HttpResponse.json({ uptime_seconds: 10860, version: "test-build" }),
    ),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route path="/libraries/:id" element={<LibraryDetailLayout />}>
            <Route index element={<LibraryDetailPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  for (const name of ["全量同步", "增量同步", "编辑配置"]) {
    expect(await screen.findByRole("button", { name })).toBeEnabled();
  }
  expect(screen.getAllByText("演示数据")).toHaveLength(3);
  expect(screen.getByText("12,860")).toBeVisible();
  expect(await screen.findByText("3 小时 1 分钟")).toBeVisible();
  expect(screen.queryByText("版本信息")).not.toBeInTheDocument();
  expect(screen.queryByText("test-build")).not.toBeInTheDocument();
  expect(screen.getByText(library.output_path)).toBeVisible();
  for (const [label, value] of [
    ["语言偏好", "简体中文"],
    ["刮削源", "MetaTube"],
    ["排除目录", "115 网盘/电影/样片"],
    ["包含子目录（递归扫描）", "已关闭"],
    ["跟随网盘目录变更", "已启用"],
    ["输出目录", library.output_path],
    ["STRM 文件", "已关闭"],
    ["NFO 信息", "已启用"],
    ["海报与封面", "已关闭"],
    ["STRM 已存在时", "覆盖"],
    ["文件编码", "UTF-8"],
    ["链接类型", "直连"],
    ["STRM 直连域名", "media.example.com"],
    ["链接格式", "文件 ID"],
  ] as const) {
    expect(
      screen.getByText(label, { selector: "dt", exact: true })
        .nextElementSibling,
    ).toHaveTextContent(value);
  }
  expect(
    screen.getByRole("link", { name: library.directory_path }),
  ).toHaveAttribute("href", "/cloud-drive?directory_id=123");
  expect(screen.queryByText("运行中")).not.toBeInTheDocument();
});

it("系统信息失败保留配置与演示数据，启停失败提供反馈", async () => {
  const user = userEvent.setup();
  server.use(
    http.get("/api/v1/libraries/1", () => HttpResponse.json(library)),
    http.get("/api/v1/system/status", () =>
      HttpResponse.json(
        { error: { code: "unavailable", message: "系统信息不可用" } },
        { status: 503 },
      ),
    ),
    http.patch("/api/v1/libraries/1/enabled", () =>
      HttpResponse.json(
        { error: { code: "unavailable", message: "暂时无法暂停媒体库" } },
        { status: 503 },
      ),
    ),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route path="/libraries/:id" element={<LibraryDetailLayout />}>
            <Route index element={<LibraryDetailPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("无法获取系统信息。")).toBeVisible();
  expect(screen.getByText(library.output_path)).toBeVisible();
  expect(screen.getAllByText("演示数据")).toHaveLength(3);
  await user.click(screen.getByRole("button", { name: "暂停媒体库" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "请求未能完成，请稍后重试。",
  );
  expect(screen.getAllByText("已启用", { exact: true })[0]).toBeVisible();
});

it("链接格式示例跟随 Tab 与域名变化，保持纯文本", async () => {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(
    screen.getByText("格式示例：https://media.example.com/电影名称.mkv"),
  ).toBeVisible();
  await user.click(screen.getByRole("tab", { name: "文件 ID" }));
  expect(
    screen.getByText("格式示例：https://media.example.com/123456789"),
  ).toBeVisible();
  const domain = screen.getByLabelText("STRM 直连域名");
  await user.type(domain, "media.local:8443");
  expect(
    screen.getByText("格式示例：https://media.local:8443/123456789"),
  ).toBeVisible();
  await user.click(screen.getByRole("tab", { name: "文件原名" }));
  expect(
    screen
      .getByText("格式示例：https://media.local:8443/电影名称.mkv")
      .closest("a"),
  ).toBeNull();
  await user.clear(domain);
  await user.type(domain, "https://bad.example/path");
  expect(
    screen.getByText("格式示例：https://media.example.com/电影名称.mkv"),
  ).toBeVisible();
});

it("响应策略必须存在且属于已知枚举", () => {
  expect(
    decodeLibrary({ ...library, strm_existing_file_policy: "overwrite" })
      .strm_existing_file_policy,
  ).toBe("overwrite");
  for (const invalid of [undefined, null, "", "rename", true]) {
    expect(() =>
      decodeLibrary({ ...library, strm_existing_file_policy: invalid }),
    ).toThrow("服务返回了无效响应");
  }
});

it("删除确认使用 alertdialog，默认聚焦取消并恢复触发按钮焦点", async () => {
  let deleted = false;
  server.use(
    http.get("/api/v1/libraries", () =>
      HttpResponse.json({ items: [library], next_cursor: null }),
    ),
    http.delete("/api/v1/libraries/1", () => {
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderPage();
  const remove = await screen.findByRole("button", {
    name: "删除",
  });
  await user.click(remove);
  const dialog = screen.getByRole("alertdialog", { name: "删除媒体库？" });
  const cancel = within(dialog).getByRole("button", { name: "取消" });
  expect(cancel).toHaveFocus();
  expect(dialog).toHaveAccessibleDescription(
    /仅删除媒体库配置，网盘及本地文件不会被删除/,
  );
  await user.click(cancel);
  await vi.waitFor(() =>
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument(),
  );
  await vi.waitFor(() => expect(remove).toHaveFocus());
  expect(deleted).toBe(false);
});

it("删除提交期间不能重复确认、取消或按 Escape 关闭", async () => {
  let requests = 0;
  let deleted = false;
  let finishDelete: (() => void) | undefined;
  const deletionReady = new Promise<void>((resolve) => {
    finishDelete = resolve;
  });
  server.use(
    http.get("/api/v1/libraries", () =>
      HttpResponse.json({ items: deleted ? [] : [library], next_cursor: null }),
    ),
    http.delete("/api/v1/libraries/1", async () => {
      requests += 1;
      await deletionReady;
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "删除" }));
  const dialog = screen.getByRole("alertdialog", { name: "删除媒体库？" });
  await user.click(within(dialog).getByRole("button", { name: "确认删除" }));
  expect(await within(dialog).findByRole("status")).toHaveTextContent(
    "正在删除配置，请稍候…",
  );
  const confirm = within(dialog).getByRole("button", { name: "正在删除…" });
  const cancel = within(dialog).getByRole("button", { name: "取消" });
  expect(confirm).toBeDisabled();
  expect(cancel).toBeDisabled();
  await user.click(confirm);
  await user.click(cancel);
  await user.keyboard("{Escape}");
  expect(dialog).toBeVisible();
  expect(requests).toBe(1);
  finishDelete?.();
  expect(
    await screen.findByText("创建媒体库，开始管理您的媒体配置。"),
  ).toBeVisible();
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
});

it("删除失败保留弹窗内错误并允许重试，重新打开不残留错误", async () => {
  let requests = 0;
  let deleted = false;
  server.use(
    http.get("/api/v1/libraries", () =>
      HttpResponse.json({ items: deleted ? [] : [library], next_cursor: null }),
    ),
    http.delete("/api/v1/libraries/1", () => {
      requests += 1;
      if (requests < 3)
        return HttpResponse.json(
          {
            error: {
              code: "service_unavailable",
              message: "error",
              request_id: "test",
            },
          },
          { status: 503 },
        );
      deleted = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderPage();
  const remove = await screen.findByRole("button", {
    name: "删除",
  });
  await user.click(remove);
  let dialog = screen.getByRole("alertdialog", { name: "删除媒体库？" });
  await user.click(within(dialog).getByRole("button", { name: "确认删除" }));
  expect(await within(dialog).findByRole("alert")).toHaveTextContent(
    "服务暂时不可用，请稍后重试。",
  );
  expect(
    within(dialog).getByRole("button", { name: "确认删除" }),
  ).toBeEnabled();
  await user.click(within(dialog).getByRole("button", { name: "确认删除" }));
  await vi.waitFor(() => expect(requests).toBe(2));
  expect(await within(dialog).findByRole("alert")).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "取消" }));
  await user.click(remove);
  dialog = screen.getByRole("alertdialog", { name: "删除媒体库？" });
  expect(within(dialog).queryByRole("alert")).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole("button", { name: "确认删除" }));
  expect(
    await screen.findByText("创建媒体库，开始管理您的媒体配置。"),
  ).toBeVisible();
  expect(requests).toBe(3);
});
