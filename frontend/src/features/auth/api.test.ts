import { delay, http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";

import { sessionExpiredEvent } from "@/lib/api/client";
import { server } from "@/test/server";

import {
  decodeBootstrap,
  decodeSession,
  getBootstrap,
  login,
  logout,
} from "./api";
import { anonymousBootstrap, authSession } from "./test-fixtures";

describe("认证 API 边界", () => {
  it.each([
    {},
    { ...anonymousBootstrap, csrf_token: "" },
    { ...anonymousBootstrap, administrator: authSession.administrator },
    { ...anonymousBootstrap, authenticated: true },
    {
      ...anonymousBootstrap,
      ...authSession,
      authenticated: true,
      initialized: false,
    },
    {
      ...anonymousBootstrap,
      ...authSession,
      authenticated: true,
      expires_at: "not-a-date",
    },
  ])("拒绝不一致或无效的 bootstrap %#", (value) => {
    expect(() => decodeBootstrap(value)).toThrow("无效响应");
  });
  it("校验会话并保留契约空值语义", () => {
    expect(decodeBootstrap(anonymousBootstrap)).toEqual(anonymousBootstrap);
    expect(decodeSession(authSession)).toEqual(authSession);
    expect(() =>
      decodeSession({ ...authSession, administrator: null }),
    ).toThrow();
  });
  it("将取消传递到请求", async () => {
    server.use(
      http.get("/api/v1/auth/bootstrap", async () => {
        await delay("infinite");
        return HttpResponse.json(anonymousBootstrap);
      }),
    );
    const controller = new AbortController();
    const pending = getBootstrap(controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
  });
  it("登录提交 CSRF 和记住登录选项，隐藏后端内部错误", async () => {
    server.use(
      http.post("/api/v1/auth/login", async ({ request }) => {
        expect(request.headers.get("X-CSRF-Token")).toBe("test-csrf");
        expect(await request.json()).toEqual({
          username: "admin",
          password: "password123",
          remember_login: false,
        });
        expect(request.credentials).toBe("same-origin");
        return HttpResponse.json(
          {
            error: {
              code: "invalid_credentials",
              message: "internal secret",
              request_id: "request-1",
            },
          },
          { status: 401 },
        );
      }),
    );
    await expect(
      login(
        { username: "admin", password: "password123", remember_login: false },
        "test-csrf",
      ),
    ).rejects.toMatchObject({
      code: "invalid_credentials",
      message: "用户名或密码错误，请重新输入。",
      requestId: "request-1",
    });
  });
  it("即使401是无效JSON也发出会话过期通知", async () => {
    const expired = vi.fn();
    window.addEventListener(sessionExpiredEvent, expired);
    server.use(
      http.post(
        "/api/v1/auth/logout",
        () => new HttpResponse("unauthorized", { status: 401 }),
      ),
    );
    await expect(logout("csrf")).rejects.toMatchObject({
      code: "invalid_response",
    });
    expect(expired).toHaveBeenCalledOnce();
    window.removeEventListener(sessionExpiredEvent, expired);
  });
  it("说明网络失败而不吞掉错误", async () => {
    server.use(http.get("/api/v1/auth/bootstrap", () => HttpResponse.error()));
    await expect(
      getBootstrap(new AbortController().signal),
    ).rejects.toMatchObject({ code: "network_error" });
  });
});
