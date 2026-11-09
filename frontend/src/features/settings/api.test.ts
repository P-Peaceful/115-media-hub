import { describe, expect, it } from "vitest";

import { decodeAccount, decodeQr, decodeSecurity } from "./api";

describe("设置接口响应校验", () => {
  it("校验安全设置及账号信息", () => {
    expect(
      decodeSecurity({ username: "admin", session_lifetime_days: 7 })
        .session_lifetime_days,
    ).toBe(7);
    expect(() =>
      decodeSecurity({ username: "admin", session_lifetime_days: "7" }),
    ).toThrow();
    expect(() => decodeAccount({ connected: false })).toThrow();
  });
  it("拒绝未知状态及外部二维码地址", () => {
    const qr = {
      id: "id",
      qr_url: "data:image/png;base64,abc",
      status: "waiting",
      expires_at: "2099-01-01T00:00:00Z",
    };
    expect(decodeQr(qr).status).toBe("waiting");
    expect(() => decodeQr({ ...qr, status: "unknown" })).toThrow();
    expect(() =>
      decodeQr({ ...qr, qr_url: "https://example.com/qr" }),
    ).toThrow();
  });
});
