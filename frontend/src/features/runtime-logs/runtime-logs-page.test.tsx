import { RouterProvider } from "@heroui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/features/auth/auth-provider";
import { server } from "@/test/server";

import type { RuntimeLogEntry } from "./api";
import { RuntimeLogsPage } from "./runtime-logs-page";

class MockEventSource {
  static instances: MockEventSource[] = [];
  readonly url: string;
  readyState = 0;
  closed = false;
  private listeners = new Map<string, Set<EventListener>>();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: EventListener) {
    const existing = this.listeners.get(type) ?? new Set();
    existing.add(listener);
    this.listeners.set(type, existing);
  }

  removeEventListener(type: string, listener: EventListener) {
    this.listeners.get(type)?.delete(listener);
  }

  emit(type: string, event: Event) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }

  close() {
    this.closed = true;
    this.readyState = 2;
  }
}

const initialLogs: RuntimeLogEntry[] = [
  {
    id: "2",
    occurred_at: "2026-11-06T02:42:58Z",
    level: "info",
    module: "sync",
    operation: "incremental_sync",
    job_id: "8721",
    request_id: "req-sync-1",
    error_code: null,
    message: "开始执行增量同步，媒体库：电影库",
    details: { library_id: "movie-1" },
  },
  {
    id: "1",
    occurred_at: "2026-11-06T02:40:11Z",
    level: "error",
    module: "upload",
    operation: "upload_sidecar",
    job_id: "8718",
    request_id: "req-upload-1",
    error_code: "upload_timeout",
    message: "文件上传失败：网络超时（3 次重试后仍失败）：poster.jpg",
    details: { attempt: 3 },
  },
];

function RouterBridge({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  return (
    <RouterProvider
      navigate={(path, options) => {
        void navigate(path, options);
      }}
    >
      {children}
    </RouterProvider>
  );
}

function renderLogsPage(initialPath = "/logs") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <RouterBridge>
          <AuthProvider>
            <RuntimeLogsPage />
          </AuthProvider>
        </RouterBridge>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  MockEventSource.instances = [];
  vi.stubGlobal("EventSource", MockEventSource);
  server.use(
    http.get("/api/v1/auth/bootstrap", () =>
      HttpResponse.json({
        initialized: true,
        authenticated: true,
        administrator: { id: "admin-1", username: "admin" },
        expires_at: "2099-01-01T00:00:00Z",
        csrf_token: "csrf-logs-token",
      }),
    ),
    http.get("/api/v1/tasks", () =>
      HttpResponse.json({
        items: [],
        next_cursor: null,
        total: 0,
        by_type: {},
        by_status: {},
      }),
    ),
    http.get("/api/v1/logs", () =>
      HttpResponse.json({ items: initialLogs, next_cursor: null }),
    ),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("RuntimeLogsPage", () => {
  it("渲染运行日志列表、错误高亮开关及任务链接", async () => {
    const user = userEvent.setup();
    renderLogsPage();

    expect(
      await screen.findByText("开始执行增量同步，媒体库：电影库"),
    ).toBeVisible();
    const errorMessage = screen.getByText(
      "文件上传失败：网络超时（3 次重试后仍失败）：poster.jpg",
    );
    const errorHighlightBox = errorMessage.closest("[data-highlighted]");
    expect(errorHighlightBox).not.toBeNull();
    expect(errorHighlightBox).toHaveAttribute("data-highlighted", "true");
    expect(screen.getByRole("link", { name: "#8721" })).toHaveAttribute(
      "href",
      "/tasks/8721",
    );

    const highlightCheckbox = screen.getByRole("checkbox", {
      name: "高亮错误",
    });
    expect(highlightCheckbox).toBeChecked();
    await user.click(highlightCheckbox);
    expect(errorHighlightBox).toHaveAttribute("data-highlighted", "false");
  });

  it("开启自动滚动时接收 SSE 新日志并置顶，关闭后断开连接", async () => {
    const user = userEvent.setup();
    renderLogsPage();

    expect(
      await screen.findByText("开始执行增量同步，媒体库：电影库"),
    ).toBeVisible();
    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));

    const stream = MockEventSource.instances[0]!;
    act(() => {
      stream.emit("open", new Event("open"));
      stream.emit(
        "log.created",
        new MessageEvent("log.created", {
          data: JSON.stringify({
            id: "3",
            occurred_at: "2026-11-06T02:43:10Z",
            level: "warn",
            module: "strm",
            operation: "generate",
            job_id: "8719",
            request_id: null,
            error_code: null,
            message: "文件已存在，跳过生成：/Movies/星际穿越 (2014).strm",
            details: {},
          }),
        }),
      );
    });

    expect(
      await screen.findByText(
        "文件已存在，跳过生成：/Movies/星际穿越 (2014).strm",
      ),
    ).toBeVisible();
    expect(screen.getByText("实时同步中")).toBeVisible();

    const rows = screen.getAllByRole("row");
    expect(rows[1]).toHaveTextContent("文件已存在，跳过生成");

    const autoScrollSwitch = screen.getByRole("switch", { name: "自动滚动" });
    await user.click(autoScrollSwitch);
    expect(stream.closed).toBe(true);
  });

  it("通过清理日志弹窗选择范围并提交 CSRF 清理请求", async () => {
    const user = userEvent.setup();
    let clearedScope = "";
    let clearedCsrf = "";
    let logsState = [...initialLogs];

    server.use(
      http.get("/api/v1/logs", () =>
        HttpResponse.json({ items: logsState, next_cursor: null }),
      ),
      http.post("/api/v1/logs/clear", async ({ request }) => {
        clearedCsrf = request.headers.get("X-CSRF-Token") ?? "";
        const payload = (await request.json()) as { scope: string };
        clearedScope = payload.scope;
        logsState = [];
        return HttpResponse.json({
          deleted_count: 2,
          scope: payload.scope,
        });
      }),
    );

    renderLogsPage();
    expect(
      await screen.findByText("开始执行增量同步，媒体库：电影库"),
    ).toBeVisible();

    await user.click(screen.getByRole("button", { name: "清理日志" }));
    expect(
      await screen.findByRole("heading", { name: "清理运行日志" }),
    ).toBeVisible();
    expect(
      screen.getByText(
        "仅清理系统运行日志，不会删除安全审计记录和任务中心历史，也不会修改宿主机日志文件。",
      ),
    ).toBeVisible();

    await user.click(screen.getByRole("radio", { name: /清空全部运行日志/ }));
    await user.click(screen.getByRole("button", { name: "确认清理" }));

    expect(await screen.findByText("已清理 2 条运行日志。")).toBeVisible();
    expect(clearedScope).toBe("all");
    expect(clearedCsrf).toBe("csrf-logs-token");
    expect(await screen.findByText("当前时间范围内暂无运行日志")).toBeVisible();
  });

  it("加载失败时展示错误提示与重试按钮", async () => {
    const user = userEvent.setup();
    let failedOnce = false;
    server.use(
      http.get("/api/v1/logs", () => {
        if (!failedOnce) {
          failedOnce = true;
          return HttpResponse.json(
            {
              error: {
                code: "service_unavailable",
                message: "unavailable",
                request_id: "req-fail-1",
              },
            },
            { status: 503 },
          );
        }
        return HttpResponse.json({ items: initialLogs, next_cursor: null });
      }),
    );

    renderLogsPage();
    expect(await screen.findByText(/请求 ID req-fail-1/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "重新加载" }));
    expect(
      await screen.findByText("开始执行增量同步，媒体库：电影库"),
    ).toBeVisible();
  });
});
