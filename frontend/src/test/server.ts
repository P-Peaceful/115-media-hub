import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";

export const server = setupServer(
  http.get("/api/v1/settings/115/details", () =>
    HttpResponse.json({
      status: "unconfigured",
      profile: null,
      space: null,
      space_status: "unavailable",
      devices: [],
      devices_status: "unavailable",
    }),
  ),
  http.get("/api/v1/system/status", () =>
    HttpResponse.json({ uptime_seconds: 93780, version: "dev" }),
  ),
  http.get("/health/ready", () => HttpResponse.json({ status: "ok" })),
);
