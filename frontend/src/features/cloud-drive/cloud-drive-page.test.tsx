import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
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
};
describe("网盘浏览", () => {
  it("浏览真实目录、打开详情并切换分页", async () => {
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const params = new URL(request.url).searchParams;
        if (params.get("cursor"))
          return HttpResponse.json({ ...child, items: [], next_cursor: null });
        return HttpResponse.json(
          params.get("directory_id") === "123" ? child : root,
        );
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "电影" }));
    await user.click(await screen.findByRole("button", { name: "影片.mp4" }));
    const detail = screen.getByRole("complementary", { name: "文件详情" });
    expect(
      within(detail).getByText("115 网盘 / 电影 / 影片.mp4"),
    ).toBeVisible();
    expect(
      within(detail).getByRole("button", { name: "跳转媒体库" }),
    ).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "下一页" }));
    expect(await screen.findByText("当前页没有文件")).toBeVisible();
    expect(screen.getByRole("button", { name: "返回第一页" })).toBeVisible();
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
      within(screen.getByRole("complementary", { name: "文件详情" })).getByRole(
        "button",
        { name: "复制路径" },
      ),
    );
    expect(await navigator.clipboard.readText()).toBe(expected);
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
    expect(
      screen.getByRole("complementary", { name: "文件详情" }),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "浏览器后退" }));
    await screen.findByRole("button", { name: "电影" });
    expect(
      screen.queryByRole("complementary", { name: "文件详情" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "浏览器前进" }));
    await screen.findByRole("button", { name: "影片.mp4" });
    expect(
      screen.queryByRole("complementary", { name: "文件详情" }),
    ).not.toBeInTheDocument();
  });
  it("刷新清除详情并从第一页重新请求", async () => {
    const requests: string[] = [];
    server.use(
      http.get("/api/v1/cloud-drive/files", ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor") ?? "";
        requests.push(cursor);
        return HttpResponse.json({
          ...child,
          next_cursor: cursor ? null : "next",
        });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "影片.mp4" });
    await user.click(screen.getByRole("button", { name: "下一页" }));
    await screen.findByRole("button", { name: "返回第一页" });
    await user.click(await screen.findByRole("button", { name: "影片.mp4" }));
    expect(
      screen.getByRole("complementary", { name: "文件详情" }),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "刷新" }));
    expect(await screen.findByRole("button", { name: "下一页" })).toBeVisible();
    expect(
      screen.queryByRole("complementary", { name: "文件详情" }),
    ).not.toBeInTheDocument();
    expect(requests).toEqual(["", "next", ""]);
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
