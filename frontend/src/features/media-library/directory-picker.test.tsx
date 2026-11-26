import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { expect, it, vi } from "vitest";

import type { DirectoryListing } from "@/features/cloud-drive/api";
import { server } from "@/test/server";

import { DirectoryPicker } from "./directory-picker";

const rootPath = [{ id: "0", name: "115 网盘" }];
const moviePath = [...rootPath, { id: "123", name: "电影" }];
const rootListing: DirectoryListing = {
  directory_id: "0",
  total: 1,
  page: 1,
  page_size: 200,
  total_pages: 1,
  path: rootPath,
  items: [
    {
      id: "123",
      name: "电影",
      is_directory: true,
      size_bytes: null,
      modified_at: null,
      file_type: "folder",
    },
  ],
  next_cursor: null,
};
const movieListing: DirectoryListing = {
  ...rootListing,
  directory_id: "123",
  total: 0,
  path: moviePath,
  items: [],
};

function renderPicker(onSelect = vi.fn()) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false, gcTime: 0 } },
        })
      }
    >
      <DirectoryPicker onSelect={onSelect} onClose={() => {}} />
    </QueryClientProvider>,
  );
}

it("进入目录时立即更新面包屑，等待响应期间路径不消失，支持键盘返回上级", async () => {
  let respond: (response: Response) => void = () => undefined;
  const response = new Promise<Response>((resolve) => {
    respond = resolve;
  });
  server.use(
    http.get("/api/v1/cloud-drive/files", ({ request }) =>
      new URL(request.url).searchParams.get("directory_id") === "0"
        ? HttpResponse.json(rootListing)
        : response,
    ),
  );
  const user = userEvent.setup();
  const onSelect = vi.fn();
  renderPicker(onSelect);
  await user.click(await screen.findByRole("button", { name: "电影" }));

  const breadcrumbs = screen.getByRole("navigation", { name: "目录路径" });
  expect(
    within(breadcrumbs).getByRole("link", { name: "115 网盘" }),
  ).toBeVisible();
  expect(within(breadcrumbs).getByText("电影")).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getByText("正在读取目录…")).toBeVisible();
  expect(screen.getByRole("button", { name: "选择当前目录" })).toBeDisabled();
  expect(onSelect).not.toHaveBeenCalled();

  respond(HttpResponse.json(movieListing));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "选择当前目录" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "选择当前目录" }));
  expect(onSelect).toHaveBeenCalledWith("123", "115 网盘/电影");

  within(breadcrumbs).getByRole("link", { name: "115 网盘" }).focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("button", { name: "电影" })).toBeVisible();
  expect(within(breadcrumbs).queryByText("电影")).not.toBeInTheDocument();
  expect(within(breadcrumbs).getByText("115 网盘")).toHaveAttribute(
    "aria-current",
    "page",
  );
});

it("目录加载失败时保留目标路径，可重试或通过面包屑返回", async () => {
  let failed = true;
  server.use(
    http.get("/api/v1/cloud-drive/files", ({ request }) => {
      if (new URL(request.url).searchParams.get("directory_id") === "0") {
        return HttpResponse.json(rootListing);
      }
      return failed
        ? HttpResponse.json(
            {
              error: {
                code: "provider_unavailable",
                message: "unavailable",
                request_id: "test",
              },
            },
            { status: 503 },
          )
        : HttpResponse.json(movieListing);
    }),
  );
  const user = userEvent.setup();
  renderPicker();
  await user.click(await screen.findByRole("button", { name: "电影" }));
  expect(await screen.findByText(/无法读取目录/)).toBeVisible();
  const breadcrumbs = screen.getByRole("navigation", { name: "目录路径" });
  expect(
    within(breadcrumbs).getByRole("link", { name: "115 网盘" }),
  ).toBeVisible();
  expect(within(breadcrumbs).getByText("电影")).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getByRole("button", { name: "选择当前目录" })).toBeDisabled();

  failed = false;
  await user.click(screen.getByRole("button", { name: "重试" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "选择当前目录" })).toBeEnabled(),
  );
  await user.click(within(breadcrumbs).getByRole("link", { name: "115 网盘" }));
  expect(await screen.findByRole("button", { name: "电影" })).toBeVisible();
});

it("缓存目录重新验证失败后不能选择旧数据", async () => {
  server.use(
    http.get("/api/v1/cloud-drive/files", () =>
      HttpResponse.json(
        {
          error: {
            code: "provider_auth_invalid",
            message: "invalid",
            request_id: "test",
          },
        },
        { status: 409 },
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["library-directory", "0", null], {
    directory_id: "0",
    total: 0,
    page: 1,
    page_size: 50,
    total_pages: 1,
    path: [{ id: "0", name: "115 网盘" }],
    items: [],
    next_cursor: null,
  });
  const onSelect = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <DirectoryPicker onSelect={onSelect} onClose={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "选择当前目录" })).toBeDisabled();
  expect(await screen.findByText(/无法读取目录/)).toBeVisible();
  expect(screen.getByRole("button", { name: "选择当前目录" })).toBeDisabled();
  expect(onSelect).not.toHaveBeenCalled();
});
