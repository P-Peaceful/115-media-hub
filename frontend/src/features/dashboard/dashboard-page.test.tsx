import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { server } from "@/test/server";

import { DashboardPage } from "./dashboard-page";

function renderPage() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false, gcTime: 0 } },
        })
      }
    >
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("总览页面", () => {
  it("显示加载状态并在检查成功后显示就绪", async () => {
    server.use(
      http.get("/health/ready", async () => {
        await delay(50);
        return HttpResponse.json({ status: "ok" });
      }),
    );
    renderPage();
    expect(screen.getByText("正在确认服务与数据库的连接状态…")).toBeVisible();
    expect(await screen.findByText("后端服务与数据库连接正常。")).toBeVisible();
  });
  it("将未来模块数据标记为示例并禁用快捷入口", async () => {
    renderPage();
    expect(await screen.findByText("后端服务与数据库连接正常。")).toBeVisible();
    expect(screen.getAllByText("示例数据").length).toBeGreaterThan(0);
    expect(await screen.findByText("1 天 2 小时 3 分钟")).toBeVisible();
    expect(screen.getByText("dev")).toBeVisible();
    expect(screen.getByRole("button", { name: /新建媒体库/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /查看任务/ })).toBeDisabled();
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
  it("系统信息异常时保留就绪状态并可重新获取", async () => {
    server.use(
      http.get("/api/v1/system/status", () =>
        HttpResponse.json({ uptime_seconds: -1, version: "dev" }),
      ),
    );
    renderPage();
    expect(await screen.findByText("后端服务与数据库连接正常。")).toBeVisible();
    expect(await screen.findAllByText("暂无数据")).toHaveLength(2);
    server.use(
      http.get("/api/v1/system/status", () =>
        HttpResponse.json({ uptime_seconds: 62, version: "v1.2.3" }),
      ),
    );
    await userEvent.click(screen.getByRole("button", { name: "重新检查" }));
    expect(await screen.findByText("1 分钟")).toBeVisible();
    expect(screen.getByText("v1.2.3")).toBeVisible();
  });
  it("按总览设计展示实时授权状态和动态容量信息", async () => {
    server.use(
      http.get("/api/v1/settings/115/details", () =>
        HttpResponse.json({
          status: "valid",
          profile: {
            user_name: "真实账号",
            user_id: "123",
            avatar: null,
            is_member: true,
            member_expires_at: null,
            member_forever: false,
          },
          space: { used_bytes: "1024", total_bytes: "2048" },
          space_status: "available",
          devices: [],
          devices_status: "available",
        }),
      ),
    );
    renderPage();
    expect(await screen.findByText("授权有效")).toBeVisible();
    expect(
      screen.getByText("当前账号已授权，可正常使用网盘服务。"),
    ).toBeVisible();
    expect(screen.getByText("最近检测")).toBeVisible();
    expect(screen.getByText("有效")).toBeVisible();
    expect(screen.getByText("1.00 KB")).toBeVisible();
    expect(screen.getByText("剩余 1.00 KB")).toBeVisible();
    expect(screen.getByText("50.0%")).toBeVisible();
    expect(
      screen.getByRole("progressbar", { name: "网盘空间使用率" }),
    ).toHaveAttribute("aria-valuenow", "50");
    expect(screen.getByRole("link", { name: "管理 115 授权" })).toHaveAttribute(
      "href",
      "/settings?tab=account",
    );
    expect(screen.queryByText(/剩余\s+\d+\s+天/)).not.toBeInTheDocument();
    expect(screen.queryByText("115 授权即将失效")).not.toBeInTheDocument();
  });
  it("系统和 115 状态卡都提供徽标与说明，再展示对应详情行", async () => {
    server.use(
      http.get("/api/v1/settings/115/details", () =>
        HttpResponse.json({
          status: "valid",
          profile: {
            user_name: "真实账号",
            user_id: "123",
            avatar: null,
            is_member: true,
            member_expires_at: null,
            member_forever: false,
          },
          space: null,
          space_status: "unavailable",
          devices: [],
          devices_status: "unavailable",
        }),
      ),
    );
    renderPage();
    await screen.findByText("后端服务与数据库连接正常。");
    await screen.findByText("当前账号已授权，可正常使用网盘服务。");
    const systemCard = screen.getByRole("region", { name: "系统状态" });
    const accountCard = screen.getByRole("region", { name: "115 连接状态" });
    const systemSummary = within(systemCard).getByRole("status");
    const accountSummary = within(accountCard).getByRole("status");

    expect(within(systemSummary).getByText("服务已就绪")).toBeVisible();
    expect(
      within(systemSummary).getByText("后端服务与数据库连接正常。"),
    ).toBeVisible();
    expect(within(accountSummary).getByText("授权有效")).toBeVisible();
    expect(
      within(accountSummary).getByText("当前账号已授权，可正常使用网盘服务。"),
    ).toBeVisible();
    expect(within(systemCard).getByText("运行时长")).toBeVisible();
    expect(within(accountCard).getByText("授权状态")).toBeVisible();
  });
  it("显示授权失效及上游不可用的不同状态", async () => {
    server.use(
      http.get("/api/v1/settings/115/details", () =>
        HttpResponse.json({
          status: "invalid",
          profile: null,
          space: null,
          space_status: "unavailable",
          devices: [],
          devices_status: "unavailable",
        }),
      ),
    );
    renderPage();
    expect(await screen.findByText("授权失效")).toBeVisible();
    expect(screen.getByRole("link", { name: "重新授权 115" })).toBeVisible();
  });
  it("上游失败与容量不可用不会显示虚构数据", async () => {
    server.use(
      http.get("/api/v1/settings/115/details", () =>
        HttpResponse.json(
          {
            error: {
              code: "service_unavailable",
              message: "private",
              request_id: "request",
            },
          },
          { status: 503 },
        ),
      ),
    );
    renderPage();
    expect(await screen.findByText("无法确认授权状态")).toBeVisible();
    expect(screen.getByText("容量暂不可用")).toBeVisible();
    expect(screen.queryByText("7.23 TB")).not.toBeInTheDocument();
  });
  it("提供网络失败提示", async () => {
    server.use(http.get("/health/ready", () => HttpResponse.error()));
    renderPage();
    expect(
      await screen.findByText("无法连接服务，请检查网络后重试。"),
    ).toBeVisible();
  });
});
