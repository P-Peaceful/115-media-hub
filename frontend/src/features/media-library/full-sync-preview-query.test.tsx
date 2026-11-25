import {
  focusManager,
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import { server } from "@/test/server";

import { useFullSyncPreview } from "./full-sync-preview-query";

const preview = {
  revision: 1,
  mode: "sample",
  stop_reason: "directory_limit",
  scan_complete: false,
  scanned_videos: 0,
  candidate_count: 0,
  failed_count: 0,
  samples: [],
  output: { status: "ready", message: "输出目录可用" },
};

function fixture() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return { client, Wrapper };
}

describe("全量同步抽样查询", () => {
  it("仅在进入、版本改变或显式刷新时读取，焦点、重连及库列表失效不重扫", async () => {
    let calls = 0;
    server.use(
      http.get("/api/v1/libraries/1/full-sync-preview", () => {
        calls++;
        return HttpResponse.json({ ...preview, revision: calls === 3 ? 2 : 1 });
      }),
    );
    const { client, Wrapper } = fixture();
    const { result, rerender } = renderHook(
      ({ revision }) => useFullSyncPreview("1", revision, true),
      {
        wrapper: Wrapper,
        initialProps: { revision: 1 },
      },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["libraries"] });
      await client.invalidateQueries();
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
    });
    expect(calls).toBe(1);
    await act(async () => {
      await result.current.refetch();
    });
    expect(calls).toBe(2);
    rerender({ revision: 2 });
    await waitFor(() => expect(result.current.data?.revision).toBe(2));
    expect(calls).toBe(3);
    focusManager.setFocused(undefined);
    client.clear();
  });

  it.each([
    {},
    { mode: "full" },
    { stop_reason: "unknown" },
    { scan_complete: true },
  ])("拒绝缺失或不一致的抽样合同 %j 且不自动重试", async (change) => {
    let calls = 0;
    server.use(
      http.get("/api/v1/libraries/1/full-sync-preview", () => {
        calls++;
        return HttpResponse.json(
          Object.keys(change).length === 0
            ? { revision: 1 }
            : { ...preview, ...change },
        );
      }),
    );
    const { client, Wrapper } = fixture();
    const { result } = renderHook(() => useFullSyncPreview("1", 1, true), {
      wrapper: Wrapper,
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect(calls).toBe(1);
    client.clear();
  });

  it("离开页面取消尚未完成的网盘抽样", async () => {
    let started = false;
    let aborted = false;
    server.use(
      http.get("/api/v1/libraries/1/full-sync-preview", async ({ request }) => {
        started = true;
        await new Promise<void>((resolve) =>
          request.signal.addEventListener(
            "abort",
            () => {
              aborted = true;
              resolve();
            },
            { once: true },
          ),
        );
        return HttpResponse.json(preview);
      }),
    );
    const { client, Wrapper } = fixture();
    const { unmount } = renderHook(() => useFullSyncPreview("1", 1, true), {
      wrapper: Wrapper,
    });
    await waitFor(() => expect(started).toBe(true));
    unmount();
    await waitFor(() => expect(aborted).toBe(true));
    client.clear();
  });
});
