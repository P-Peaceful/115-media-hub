import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { server } from "@/test/server";

import {
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
  description: "",
  directory_id: "123",
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
    const dialog = screen.getByRole("dialog", { name: "删除媒体库？" });
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
  await user.type(screen.getByLabelText("媒体库名称 *"), "我的电影");
  await user.type(
    screen.getByLabelText("输出目录（本地） *"),
    "/app/media/电影",
  );
  await user.click(screen.getByRole("radio", { name: "电视剧" }));
  await user.click(screen.getByRole("switch", { name: "生成 STRM 文件" }));
  await user.click(screen.getByRole("button", { name: /链接类型/ }));
  await screen.findByRole("option", { name: "代理链接" });
  await user.keyboard("{End}{Enter}");
  await user.click(screen.getByRole("button", { name: /选择绑定目录/ }));
  await user.click(await screen.findByRole("button", { name: "选择当前目录" }));
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  await vi.waitFor(() =>
    expect(submitted).toMatchObject({
      name: "我的电影",
      directory_id: "0",
      directory_path: "115 网盘",
      output_path: "/app/media/电影",
      enabled: true,
      media_type: "tv",
      generate_strm: false,
      link_type: "proxy",
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
  const directory = screen.getByRole("button", { name: /选择绑定目录/ });
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(await screen.findByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  expect(directory).toHaveAccessibleDescription("请选择绑定的 115 网盘目录。");
  expect(submitted).toBeUndefined();
  await user.type(screen.getByLabelText("媒体库名称 *"), "我的电影");
  await user.type(
    screen.getByLabelText("输出目录（本地） *"),
    "/app/media/电影",
  );
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(screen.getByText("请选择绑定的 115 网盘目录。")).toBeVisible();
  expect(submitted).toBeUndefined();
  await user.click(directory);
  await user.click(await screen.findByRole("button", { name: "选择当前目录" }));
  expect(
    screen.queryByText("请选择绑定的 115 网盘目录。"),
  ).not.toBeInTheDocument();
  expect(directory).toHaveAccessibleDescription("115 网盘");
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
  await user.type(screen.getByLabelText("输出目录（本地） *"), "relative");
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(screen.getByLabelText("媒体库名称 *")).toHaveAccessibleDescription(
    "请输入媒体库名称。",
  );
  expect(screen.getByLabelText("媒体库名称 *")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByText("请输入媒体库名称。")).toBeVisible();
  expect(
    screen.getByLabelText("输出目录（本地） *"),
  ).toHaveAccessibleDescription("请输入以 / 开头的本地绝对路径。");
  expect(screen.getByText("请输入以 / 开头的本地绝对路径。")).toBeVisible();
});

it("路径映射和文件大小错误关联对应输入", async () => {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <LibraryFormPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: /添加路径映射/ }));
  const size = screen.getByLabelText("过滤小文件（MB）");
  await user.clear(size);
  await user.type(size, "1048577");
  await user.click(screen.getByRole("button", { name: "创建媒体库" }));
  expect(screen.getByLabelText("映射 1 源路径")).toHaveAccessibleDescription(
    "请输入不超过 1024 个字符的源路径。",
  );
  expect(screen.getByLabelText("映射 1 目标路径")).toHaveAccessibleDescription(
    "请输入不超过 1024 个字符的目标路径。",
  );
  expect(size).toHaveAccessibleDescription("请输入 0 到 1048576 之间的整数。");
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
  await user.click(screen.getByRole("button", { name: /选择绑定目录/ }));
  expect(await screen.findByText(/无法读取目录/)).toBeVisible();
  expect(screen.getByRole("button", { name: "重试" })).toBeVisible();
});

it("详情中执行操作不可用且不显示虚构统计", async () => {
  server.use(http.get("/api/v1/libraries/1", () => HttpResponse.json(library)));
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route path="/libraries/:id" element={<LibraryDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(
    await screen.findByRole("button", { name: "全量同步" }),
  ).toBeDisabled();
  expect(screen.getByRole("button", { name: "增量同步" })).toBeDisabled();
  expect(screen.getAllByText("暂无数据")).toHaveLength(2);
});
