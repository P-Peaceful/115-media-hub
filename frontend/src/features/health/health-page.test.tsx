import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { server } from "@/test/server";

import { HealthPage } from "./health-page";

function renderPage() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false, gcTime: 0 } },
        })
      }
    >
      <HealthPage />
    </QueryClientProvider>,
  );
}

describe("服务状态页面", () => {
  it("显示加载状态并在检查成功后显示就绪", async () => {
    server.use(
      http.get("/health/ready", async () => {
        await delay(50);
        return HttpResponse.json({ status: "ok" });
      }),
    );
    renderPage();
    expect(screen.getByRole("status")).toHaveTextContent("正在确认");
    expect(await screen.findByText("后端服务与数据库连接正常。")).toBeVisible();
  });
  it("显示安全错误和请求编号，并允许重新检查", async () => {
    server.use(
      http.get("/health/ready", () =>
        HttpResponse.json(
          {
            error: {
              code: "service_unavailable",
              message: "private internals",
              request_id: "test-request",
            },
          },
          { status: 503 },
        ),
      ),
    );
    renderPage();
    expect(await screen.findByText("服务暂未就绪，请稍后重试。")).toBeVisible();
    expect(screen.queryByText("private internals")).not.toBeInTheDocument();
    expect(screen.getByText("请求编号：test-request")).toBeVisible();
    server.use(
      http.get("/health/ready", () => HttpResponse.json({ status: "ok" })),
    );
    await userEvent.click(screen.getByRole("button", { name: "重新检查" }));
    expect(await screen.findByText("后端服务与数据库连接正常。")).toBeVisible();
  });
  it.each([
    HttpResponse.json({ status: "unexpected" }),
    new HttpResponse("<html>bad gateway</html>"),
  ])("拒绝无效服务响应 %#", async (response) => {
    server.use(http.get("/health/ready", () => response));
    renderPage();
    expect(
      await screen.findByText("服务返回了无效响应，请稍后重试。"),
    ).toBeVisible();
  });
  it("提供网络失败提示", async () => {
    server.use(http.get("/health/ready", () => HttpResponse.error()));
    renderPage();
    expect(
      await screen.findByText("无法连接服务，请检查网络后重试。"),
    ).toBeVisible();
  });
});
