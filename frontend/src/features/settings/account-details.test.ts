import { describe, expect, it } from "vitest";

import { decodeAccountDetails, formatBytes } from "./account-details";

const valid = {
  status: "valid",
  profile: {
    user_name: "网盘用户",
    user_id: "123456789012345678",
    avatar: null,
    is_member: true,
    member_expires_at: "2023-11-14T22:13:20Z",
    member_forever: false,
  },
  space: { used_bytes: "1099511627776", total_bytes: "2199023255552" },
  space_status: "available",
  devices: [
    {
      name: "手机",
      type: "iOS",
      ip: "127.0.0.1",
      city: "上海",
      login_at: null,
      is_current: true,
    },
  ],
  devices_status: "available",
};
describe("115 账号详情", () => {
  it("保留字符串 ID 与大容量并格式化字节", () => {
    expect(decodeAccountDetails(valid).profile?.user_id).toBe(
      "123456789012345678",
    );
    expect(decodeAccountDetails(valid).devices[0]?.is_current).toBe(true);
    expect(formatBytes(valid.space.used_bytes)).toBe("1.00 TB");
    expect(formatBytes(null)).toBe("暂无数据");
  });
  it("区分无授权、空间不可用和格式错误", () => {
    expect(
      decodeAccountDetails({
        status: "unconfigured",
        profile: null,
        space: null,
        space_status: "unavailable",
        devices: [],
        devices_status: "unavailable",
      }).status,
    ).toBe("unconfigured");
    expect(
      decodeAccountDetails({
        ...valid,
        space: null,
        space_status: "unavailable",
        devices: [],
        devices_status: "unavailable",
      }).space,
    ).toBeNull();
    expect(() =>
      decodeAccountDetails({
        ...valid,
        space: { used_bytes: "NaN", total_bytes: "2" },
      }),
    ).toThrow();
    expect(() => decodeAccountDetails({ ...valid, profile: null })).toThrow();
  });
});
