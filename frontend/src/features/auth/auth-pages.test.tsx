import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it } from "vitest";

import { sessionExpiredEvent } from "@/lib/api/client";
import { server } from "@/test/server";

import { AuthBoundary } from "./auth-boundary";
import { AuthProvider } from "./auth-provider";
import { LoginPage } from "./login-page";
import { SetupPage } from "./setup-page";
import { anonymousBootstrap, authSession } from "./test-fixtures";

function renderAuth(path = "/") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <AuthProvider>
          <Routes>
            <Route element={<AuthBoundary />}>
              <Route path="/login" element={<LoginPage />} />
              <Route path="/setup" element={<SetupPage />} />
              <Route path="*" element={<h1>受保护页面</h1>} />
            </Route>
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}

beforeEach(() => {
  server.use(
    http.get("/api/v1/auth/bootstrap", () =>
      HttpResponse.json(anonymousBootstrap),
    ),
  );
});

describe("认证页面与路由", () => {
  it("服务器字段错误关联输入，网络失败允许重新提交", async () => {
    server.use(
      http.post("/api/v1/auth/login", () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "safe",
              request_id: "field-error",
              fields: { username: "该账号格式无效。" },
            },
          },
          { status: 400 },
        ),
      ),
    );
    renderAuth("/login");
    await screen.findByRole("heading", { name: "管理员登录" });
    await userEvent.type(screen.getByLabelText("用户名"), "admin");
    await userEvent.type(screen.getByLabelText("密码"), "password123");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByText("该账号格式无效。")).toBeVisible();
    expect(screen.getByLabelText("用户名")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    await userEvent.type(screen.getByLabelText("用户名"), "2");
    server.use(http.post("/api/v1/auth/login", () => HttpResponse.error()));
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("无法连接服务");
    expect(screen.getByRole("button", { name: "登录" })).toBeEnabled();
  });
  it("受保护401取消在途查询，迟到响应无法恢复缓存", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json({
          ...anonymousBootstrap,
          ...authSession,
          authenticated: true,
        }),
      ),
    );
    const client = renderAuth();
    await screen.findByRole("heading", { name: "受保护页面" });
    let finish = () => {};
    const pending = client
      .fetchQuery({
        queryKey: ["private-pending"],
        queryFn: () =>
          new Promise<string>((resolve) => {
            finish = () => resolve("late-private-data");
          }),
      })
      .catch(() => undefined);
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json(anonymousBootstrap),
      ),
    );
    await act(async () => {
      window.dispatchEvent(new Event(sessionExpiredEvent));
      await Promise.resolve();
    });
    await screen.findByRole("heading", { name: "管理员登录" });
    finish();
    await pending;
    expect(client.getQueryData(["private-pending"])).toBeUndefined();
  });
  it("页面重新获得焦点时发现撤销会话并清理缓存", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json({
          ...anonymousBootstrap,
          ...authSession,
          authenticated: true,
        }),
      ),
    );
    const client = renderAuth();
    await screen.findByRole("heading", { name: "受保护页面" });
    client.setQueryData(["private"], "secret");
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json(anonymousBootstrap),
      ),
    );
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await Promise.resolve();
    });
    await screen.findByRole("heading", { name: "管理员登录" });
    expect(client.getQueryData(["private"])).toBeUndefined();
    focusManager.setFocused(undefined);
  });

  it("显示安全加载状态并在网络失败后允许重试", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", async () => {
        await delay(40);
        return HttpResponse.error();
      }),
    );
    renderAuth();
    expect(screen.getByRole("status")).toHaveTextContent("正在确认登录状态");
    expect(await screen.findByRole("alert")).toHaveTextContent("无法连接服务");
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json(anonymousBootstrap),
      ),
    );
    await userEvent.click(screen.getByRole("button", { name: "重新连接" }));
    expect(
      await screen.findByRole("heading", { name: "管理员登录" }),
    ).toBeVisible();
  });
  it("显示错误凭据和安全消息，随后登录清除受保护缓存", async () => {
    let success = false;
    server.use(
      http.post("/api/v1/auth/login", () =>
        success
          ? HttpResponse.json(authSession)
          : HttpResponse.json(
              {
                error: {
                  code: "invalid_credentials",
                  message: "do not expose",
                  request_id: "login-test",
                },
              },
              { status: 401 },
            ),
      ),
    );
    const client = renderAuth("/login?return_to=/libraries");
    await screen.findByRole("heading", { name: "管理员登录" });
    client.setQueryData(["libraries"], ["old-private-data"]);
    await userEvent.type(screen.getByLabelText("用户名"), "admin");
    await userEvent.type(screen.getByLabelText("密码"), "password123");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "用户名或密码错误",
    );
    expect(screen.queryByText("do not expose")).not.toBeInTheDocument();
    success = true;
    await userEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(
      await screen.findByRole("heading", { name: "受保护页面" }),
    ).toBeVisible();
    expect(client.getQueryData(["libraries"])).toBeUndefined();
  });
  it("密码可显示并提供本地恢复命令和关闭焦点恢复", async () => {
    renderAuth("/login");
    await screen.findByRole("heading", { name: "管理员登录" });
    expect(screen.getByLabelText("密码")).toHaveAttribute("type", "password");
    await userEvent.click(screen.getByRole("button", { name: "显示密码" }));
    expect(screen.getByLabelText("密码")).toHaveAttribute("type", "text");
    const forgot = screen.getByRole("button", { name: "忘记密码？" });
    await userEvent.click(forgot);
    expect(await screen.findByRole("dialog")).toHaveTextContent(
      "mediahub admin reset-password",
    );
    await userEvent.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    await waitFor(() => expect(forgot).toHaveFocus());
  });
  it("首次运行显示已确认默认值并阻止确认密码不一致", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json({ ...anonymousBootstrap, initialized: false }),
      ),
    );
    renderAuth();
    await screen.findByRole("heading", { name: "首次使用 · 系统初始化" });
    expect(screen.getByLabelText("输出目录")).toHaveValue("/app/media");
    await userEvent.type(screen.getByLabelText("管理员账号"), "admin");
    await userEvent.type(screen.getByLabelText("设置密码"), "password123");
    await userEvent.type(screen.getByLabelText("确认密码"), "different123");
    await userEvent.click(screen.getByRole("button", { name: "开始初始化" }));
    expect(await screen.findByText("两次输入的密码不一致。")).toBeVisible();
  });
  it("过期定时器阻止已打开页面继续显示身份并清理缓存", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json({
          ...anonymousBootstrap,
          ...authSession,
          authenticated: true,
          expires_at: new Date(Date.now() + 250).toISOString(),
        }),
      ),
    );
    const client = renderAuth();
    expect(
      await screen.findByRole("heading", { name: "受保护页面" }),
    ).toBeVisible();
    client.setQueryData(["private"], "sensitive");
    server.use(
      http.get("/api/v1/auth/bootstrap", () =>
        HttpResponse.json(anonymousBootstrap),
      ),
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 300));
    });
    expect(
      await screen.findByRole("heading", { name: "管理员登录" }),
    ).toBeVisible();
    expect(client.getQueryData(["private"])).toBeUndefined();
  });
});
