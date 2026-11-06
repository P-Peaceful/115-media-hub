import { delay, http, HttpResponse } from "msw";
import { expect, it } from "vitest";

import { server } from "@/test/server";

import { getReadiness } from "./api";

it("将调用方的取消传递至健康检查请求", async () => {
  server.use(
    http.get("/health/ready", async () => {
      await delay("infinite");
      return HttpResponse.json({ status: "ok" });
    }),
  );
  const controller = new AbortController();
  const result = getReadiness(controller.signal);
  controller.abort();
  await expect(result).rejects.toMatchObject({ name: "AbortError" });
});
