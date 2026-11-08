import { describe, expect, it } from "vitest";

import { decodeSystemStatus } from "./system-status";

describe("decodeSystemStatus", () => {
  it.each([" ", "v".repeat(129)])("保留符合契约的版本字符串 %#", (version) => {
    expect(decodeSystemStatus({ uptime_seconds: 0, version })).toEqual({
      uptime_seconds: 0,
      version,
    });
  });

  it("拒绝空版本字符串", () => {
    expect(() =>
      decodeSystemStatus({ uptime_seconds: 0, version: "" }),
    ).toThrow();
  });
});
