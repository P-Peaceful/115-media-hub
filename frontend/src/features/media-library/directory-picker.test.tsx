import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { expect, it, vi } from "vitest";

import { server } from "@/test/server";

import { DirectoryPicker } from "./directory-picker";

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
