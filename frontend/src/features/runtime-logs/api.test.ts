import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";

import { sessionExpiredEvent } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import { server } from "@/test/server";

import {
  buildRuntimeLogStreamUrl,
  clearRuntimeLogs,
  decodeRuntimeLogClearedEvent,
  decodeRuntimeLogCreatedEvent,
  exportRuntimeLogs,
  listRuntimeLogs,
} from "./api";

const sampleEntry = {
  id: "101",
  occurred_at: "2026-11-06T02:42:58Z",
  level: "info" as const,
  module: "sync" as const,
  operation: "incremental_sync",
  job_id: "8721",
  request_id: "req-1",
  error_code: null,
  message: "开始执行增量同步，媒体库：电影库",
  details: { library_id: "lib-1" },
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("runtime-logs api", () => {
  it("查询并校验运行日志分页数据", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/v1/logs", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({
          items: [sampleEntry],
          next_cursor: "cursor-2",
        });
      }),
    );

    const page = await listRuntimeLogs(
      {
        range: "24h",
        level: "info",
        module: "sync",
        job_id: "8721",
        q: "电影库",
        limit: 50,
      },
      new AbortController().signal,
    );

    expect(page.items).toHaveLength(1);
    expect(page.items[0]?.id).toBe("101");
    expect(page.next_cursor).toBe("cursor-2");
    const params = new URL(capturedUrl).searchParams;
    expect(params.get("range")).toBe("24h");
    expect(params.get("level")).toBe("info");
    expect(params.get("module")).toBe("sync");
    expect(params.get("job_id")).toBe("8721");
    expect(params.get("q")).toBe("电影库");
    expect(params.get("limit")).toBe("50");
  });

  it("拒绝非法日志负载并抛出 invalid_response", async () => {
    server.use(
      http.get("/api/v1/logs", () =>
        HttpResponse.json({
          items: [{ ...sampleEntry, level: "fatal" }],
          next_cursor: null,
        }),
      ),
    );

    await expect(
      listRuntimeLogs({ range: "24h" }, new AbortController().signal),
    ).rejects.toMatchObject({ code: "invalid_response" });
  });

  it("携带 CSRF 令牌执行日志清理并解析结果", async () => {
    let csrfHeader = "";
    let body: unknown = null;
    server.use(
      http.post("/api/v1/logs/clear", async ({ request }) => {
        csrfHeader = request.headers.get("X-CSRF-Token") ?? "";
        body = await request.json();
        return HttpResponse.json({ deleted_count: 42, scope: "before_7d" });
      }),
    );

    const result = await clearRuntimeLogs("csrf-token-1", "before_7d");
    expect(csrfHeader).toBe("csrf-token-1");
    expect(body).toEqual({ scope: "before_7d" });
    expect(result).toEqual({ deleted_count: 42, scope: "before_7d" });
  });

  it("下载日志时解析文件名与截断头", async () => {
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:runtime-logs");
    const revokeObjectURL = vi
      .spyOn(URL, "revokeObjectURL")
      .mockImplementation(() => undefined);
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined);

    server.use(
      http.get("/api/v1/logs/export", ({ request }) => {
        const params = new URL(request.url).searchParams;
        expect(params.get("format")).toBe("jsonl");
        expect(params.get("range")).toBe("7d");
        return new HttpResponse('{"id":"101"}\n', {
          status: 200,
          headers: {
            "Content-Type": "application/x-ndjson; charset=utf-8",
            "Content-Disposition":
              'attachment; filename="mediahub-logs-20261106.jsonl"',
            "X-Export-Truncated": "true",
          },
        });
      }),
    );

    const result = await exportRuntimeLogs({ range: "7d" }, "jsonl");
    expect(result).toEqual({
      filename: "mediahub-logs-20261106.jsonl",
      truncated: true,
    });
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(click).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).not.toThrow();
  });

  it("下载遇到 401 时触发会话过期事件并抛出 ApiError", async () => {
    const expired = vi.fn();
    window.addEventListener(sessionExpiredEvent, expired);
    server.use(
      http.get("/api/v1/logs/export", () =>
        HttpResponse.json(
          {
            error: {
              code: "authentication_required",
              message: "请先登录",
              request_id: "req-401",
            },
          },
          { status: 401 },
        ),
      ),
    );

    try {
      await expect(
        exportRuntimeLogs({ range: "24h" }, "log"),
      ).rejects.toBeInstanceOf(ApiError);
      expect(expired).toHaveBeenCalledTimes(1);
    } finally {
      window.removeEventListener(sessionExpiredEvent, expired);
    }
  });

  it("构造 SSE 流地址并安全解码事件", () => {
    expect(
      buildRuntimeLogStreamUrl({
        range: "24h",
        level: "error",
        module: "strm",
        job_id: "8719",
        q: "星际穿越",
      }),
    ).toBe(
      "/api/v1/logs/stream?level=error&module=strm&job_id=8719&q=%E6%98%9F%E9%99%85%E7%A9%BF%E8%B6%8A",
    );

    const valid = new MessageEvent("log.created", {
      data: JSON.stringify(sampleEntry),
    });
    expect(decodeRuntimeLogCreatedEvent(valid)?.id).toBe("101");

    const invalid = new MessageEvent("log.created", {
      data: JSON.stringify({ id: "" }),
    });
    expect(decodeRuntimeLogCreatedEvent(invalid)).toBeNull();

    const cleared = new MessageEvent("log.cleared", {
      data: JSON.stringify({ scope: "all" }),
    });
    expect(decodeRuntimeLogClearedEvent(cleared)).toBe(true);
  });
});
