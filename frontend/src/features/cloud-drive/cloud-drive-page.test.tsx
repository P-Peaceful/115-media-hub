import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter, useNavigate } from "react-router";
import { describe, expect, it } from "vitest";

import { server } from "@/test/server";

import { CloudDrivePage } from "./cloud-drive-page";

function HistoryControls() {
  const navigate = useNavigate();
  return (
    <>
      <button
        onClick={() => {
          void navigate(-1);
        }}
      >
        浏览器后退
      </button>
      <button
        onClick={() => {
          void navigate(1);
        }}
      >
        浏览器前进
      </button>
    </>
  );
}
function renderPage(
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  }),
  initialEntry = "/",
) {
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <CloudDrivePage />
        <HistoryControls />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
const root = {
  directory_id: "0",
  path: [{ id: "0", name: "115 网盘" }],
  items: [
    {
      id: "123",
      name: "电影",
      is_directory: true,
      size_bytes: null,
      modified_at: null,
      file_type: "directory",
    },
  ],
  next_cursor: null,
  total: 1,
  page: 1,
  page_size: 50,
  total_pages: 1,
};
const child = {
  directory_id: "123",
  path: [...root.path, { id: "123", name: "电影" }],
  items: [
    {
      id: "456",
      name: "影片.mp4",
      is_directory: false,
      size_bytes: "1024",
      modified_at: null,
      file_type: "video",
    },
  ],
  next_cursor: "next",
  total: 51,
  page: 1,
  page_size: 50,
  total_pages: 2,
};
describe("网盘浏览", () => {
  it("旧游标返回未满页时继续使用游标，避免跳过项目", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const params = new URL(request.url).searchParams;
        requests.push(params);
        return HttpResponse.json({
          ...child,
          total: 101,
          total_pages: 3,
          page: params.get("cursor") === "aligned" ? 2 : null,
          next_cursor: params.get("cursor") === "aligned" ? "next" : null,
        });
      }),
    );
    const user = userEvent.setup();
    renderPage(undefined, "/?cursor=aligned");
    await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(screen.getByRole("button", { name: "下一页" }));
    await waitFor(() => expect(requests.at(-1)?.get("cursor")).toBe("next"));
    expect(requests.at(-1)?.has("page")).toBe(false);
  });
  it("浏览真实目录、打开详情并切换分页", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const params = new URL(request.url).searchParams;
        if (params.get("page") === "2")
          return HttpResponse.json({
            ...child,
            items: [],
            next_cursor: null,
            total: 1,
            total_pages: 1,
            page: 2,
          });
        return HttpResponse.json(
          params.get("directory_id") === "123" ? child : root,
        );
      }),
    );
    const user = userEvent.setup();
    renderPage();
    const firstPager = await screen.findByRole("navigation", {
      name: "目录分页",
    });
    expect(within(firstPager).getByText("总计 1 项")).toBeVisible();
    expect(within(firstPager).getByText("本页 1 项")).toBeVisible();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    await user.click(await screen.findByRole("button", { name: "影片.mp4" }));
    const detail = screen.getByRole("dialog", { name: "文件详情" });
    expect(
      within(detail).getByText("115 网盘 / 电影 / 影片.mp4"),
    ).toBeVisible();
    expect(
      within(detail).getByRole("button", { name: "跳转媒体库" }),
    ).toBeDisabled();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "下一页" }));
    expect(await screen.findByText("当前页没有文件")).toBeVisible();
    const lastPager = screen.getByRole("navigation", { name: "目录分页" });
    expect(within(lastPager).getByText("总计 1 项")).toBeVisible();
    expect(within(lastPager).getByText("本页 0 项")).toBeVisible();
    expect(within(lastPager).getByText(/目录已变化/)).toBeVisible();
    expect(screen.getByRole("button", { name: "上一页" })).toBeEnabled();
  });
  it.each([
    { path: [{ id: "0", name: "115 网盘" }], expected: "/ 影片 文件.mp4 " },
    {
      path: [
        { id: "0", name: "115 网盘" },
        { id: "123", name: "115 网盘" },
        { id: "124", name: "电影 资源" },
      ],
      expected: "/115 网盘/电影 资源/ 影片 文件.mp4 ",
    },
  ])("详情和行操作复制不含根标签的路径 %#", async ({ path, expected }) => {
    const item = {
      ...child.items[0],
      id: "456",
      name: " 影片 文件.mp4 ",
      is_directory: false,
      size_bytes: "1024",
      modified_at: null,
      file_type: "video",
    };
    server.use(
      http.get("/api/v1/cloud-drive/files", () =>
        HttpResponse.json({
          ...child,
          path,
          items: [item],
        }),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "影片 文件.mp4" }),
    );
    await user.click(
      within(screen.getByRole("dialog", { name: "文件详情" })).getByRole(
        "button",
        { name: "复制路径" },
      ),
    );
    expect(await navigator.clipboard.readText()).toBe(expected);
    await user.keyboard("{Escape}");
    await user.click(
      screen.getByRole("button", { name: /影片 文件.mp4.*操作/ }),
    );
    await user.click(screen.getByRole("menuitem", { name: "复制路径" }));
    expect(await navigator.clipboard.readText()).toBe(expected);
  });
  it("浏览器历史切换时不会恢复旧详情", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) =>
        HttpResponse.json(
          new URL(request.url).searchParams.get("directory_id") === "123"
            ? child
            : root,
        ),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    await user.click(await screen.findByRole("button", { name: "影片.mp4" }));
    expect(screen.getByRole("dialog", { name: "文件详情" })).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: "浏览器后退", hidden: true }),
    );
    await screen.findByRole("button", { name: "电影" });
    expect(
      screen.queryByRole("dialog", { name: "文件详情" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "浏览器前进" }));
    await screen.findByRole("button", { name: "影片.mp4" });
    expect(
      screen.queryByRole("dialog", { name: "文件详情" }),
    ).not.toBeInTheDocument();
  });
  it("刷新清除详情并从第一页重新请求", async () => {
    const requests: string[] = [];
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const pageNumber = Number(
          new URL(request.url).searchParams.get("page") ?? "1",
        );
        requests.push(String(pageNumber));
        return HttpResponse.json({
          ...child,
          page: pageNumber,
          next_cursor: pageNumber > 1 ? null : "next",
        });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(screen.getByRole("button", { name: "下一页" }));
    await screen.findByRole("button", { name: "上一页" });
    await user.click(await screen.findByRole("button", { name: "影片.mp4" }));
    expect(screen.getByRole("dialog", { name: "文件详情" })).toBeVisible();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "刷新" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "下一页" })).toBeEnabled(),
    );
    expect(
      screen.queryByRole("dialog", { name: "文件详情" }),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(requests).toEqual(["1", "2", "1"]));
  });
  it("慢请求期间显示目标路径，失败后不把旧路径当作当前目录", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", async ({ request }) => {
        if (new URL(request.url).searchParams.get("directory_id") === "123") {
          await delay(200);
          return HttpResponse.json(
            {
              error: {
                code: "not_found",
                message: "missing",
                request_id: "test",
              },
            },
            { status: 404 },
          );
        }
        return HttpResponse.json(root);
      }),
    );
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByText("总计 1 项")).toBeVisible();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    const path = screen.getByRole("navigation", { name: "目录路径" });
    expect(within(path).getByText("115 网盘")).toBeVisible();
    expect(within(path).getByText("电影")).toBeVisible();
    expect(await screen.findByText("当前目录不存在或已被移除")).toBeVisible();
    expect(within(path).getByText("目录路径暂不可用")).toBeVisible();
    expect(within(path).queryByText("电影")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "浏览器后退" }));
    expect(await screen.findByRole("button", { name: "电影" })).toBeVisible();
    expect(
      within(path).queryByText("目录路径暂不可用"),
    ).not.toBeInTheDocument();
  });
  it("连续切换目录时忽略迟到的旧响应", async () => {
    const other = { ...root.items[0], id: "789", name: "剧集" };
    server.use(
      http.get("/api/v1/cloud-drive/files", async ({ request }) => {
        const id = new URL(request.url).searchParams.get("directory_id");
        if (id === "123") {
          await delay(300);
          return HttpResponse.json(child);
        }
        if (id === "789") {
          await delay(70);
          return HttpResponse.json({
            directory_id: "789",
            path: [...root.path, { id: "789", name: "剧集" }],
            items: [],
            total: 0,
            page: 1,
            page_size: 50,
            total_pages: 1,
            next_cursor: null,
          });
        }
        return HttpResponse.json({
          ...root,
          items: [...root.items, other],
          total: 2,
        });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    expect(
      within(screen.getByRole("navigation", { name: "目录路径" })).getByText(
        "电影",
      ),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "浏览器后退" }));
    await user.click(await screen.findByRole("button", { name: "剧集" }));
    expect(
      within(screen.getByRole("navigation", { name: "目录路径" })).getByText(
        "剧集",
      ),
    ).toBeVisible();
    expect(await screen.findByText("当前目录为空")).toBeVisible();
    await delay(350);
    const path = screen.getByRole("navigation", { name: "目录路径" });
    expect(within(path).getByText("剧集")).toBeVisible();
    expect(within(path).queryByText("电影")).not.toBeInTheDocument();
    expect(screen.getByText("总计 0 项")).toBeVisible();
  });
  it("返回缓存目录后重新验证失败时，不继续展示旧路径与文件", async () => {
    let childRequests = 0;
    server.use(
      http.get("/api/v1/cloud-drive/files", async ({ request }) => {
        if (new URL(request.url).searchParams.get("directory_id") !== "123") {
          return HttpResponse.json(root);
        }
        childRequests += 1;
        if (childRequests === 1) return HttpResponse.json(child);
        await delay(100);
        return HttpResponse.json(
          {
            error: {
              code: "not_found",
              message: "missing",
              request_id: "test",
            },
          },
          { status: 404 },
        );
      }),
    );
    const user = userEvent.setup();
    renderPage(
      new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    );
    await user.click(await screen.findByRole("button", { name: "电影" }));
    await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(screen.getByRole("button", { name: "浏览器后退" }));
    await screen.findByRole("button", { name: "电影" });
    await user.click(screen.getByRole("button", { name: "浏览器前进" }));
    expect(await screen.findByText("当前目录不存在或已被移除")).toBeVisible();
    expect(screen.getByText("目录路径暂不可用")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "影片.mp4" }),
    ).not.toBeInTheDocument();
  });
  it("切换视图并将排序和过滤传递给目录接口", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const requested = new URL(request.url).searchParams;
        requests.push(requested);
        return HttpResponse.json({
          ...child,
          page: Number(requested.get("page") ?? "1"),
        });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(screen.getByRole("button", { name: "网格视图" }));
    const gridPager = screen.getByRole("navigation", { name: "目录分页" });
    expect(within(gridPager).getByText("总计 51 项")).toBeVisible();
    expect(within(gridPager).getByText("本页 1 项")).toBeVisible();
    expect(screen.getByRole("button", { name: "网格视图" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await user.click(
      within(gridPager).getByRole("button", { name: "第 2 页" }),
    );
    await waitFor(() => expect(requests.at(-1)?.get("page")).toBe("2"));
    await user.click(screen.getByRole("button", { name: "大小排序" }));
    await screen.findByRole("button", { name: "大小排序，升序" });
    await waitFor(() => expect(requests.at(-1)?.get("sort")).toBe("size"));
    expect(requests.at(-1)?.get("page")).toBe("1");
    await user.click(screen.getByRole("button", { name: /文件类型/ }));
    await user.click(screen.getByRole("option", { name: "视频文件" }));
    await screen.findByRole("button", { name: /视频文件.*文件类型/ });
    expect(requests.at(-1)?.get("type")).toBe("video");
  });
  it("面包屑返回父目录，详情弹窗支持键盘关闭", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) =>
        HttpResponse.json(
          new URL(request.url).searchParams.get("directory_id") === "123"
            ? child
            : root,
        ),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    const trigger = await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(trigger);
    expect(screen.getByRole("dialog", { name: "文件详情" })).toBeVisible();
    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: "文件详情" }),
    ).not.toBeInTheDocument();
    await user.click(
      within(screen.getByRole("navigation", { name: "目录路径" })).getByRole(
        "link",
        { name: "115 网盘" },
      ),
    );
    expect(await screen.findByRole("button", { name: "电影" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "浏览器后退" }));
    expect(
      await screen.findByRole("button", { name: "影片.mp4" }),
    ).toBeVisible();
  });
  it("区分未授权与服务错误", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", () =>
        HttpResponse.json(
          {
            error: {
              code: "provider_not_configured",
              message: "",
              request_id: "test",
            },
          },
          { status: 409 },
        ),
      ),
    );
    renderPage();
    expect(await screen.findByText("尚未授权 115 网盘")).toBeVisible();
    expect(screen.getByRole("link", { name: "管理网盘授权" })).toHaveAttribute(
      "href",
      "/settings?tab=account",
    );
  });
});
