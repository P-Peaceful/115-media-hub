import { describe, expect, it } from "vitest";

import { decodeListing } from "./api";

const page = {
  directory_id: "0",
  path: [],
  items: [],
  next_cursor: null,
  total: 0,
  page: 1,
  page_size: 50,
  total_pages: 1,
};

describe("目录响应总数校验", () => {
  it("保留合法的零和非零总数", () => {
    expect(decodeListing(page).total).toBe(0);
    expect(decodeListing({ ...page, total: 30 }).total).toBe(30);
  });
  it.each([undefined, null, -1, 1.5, "1", Number.MAX_SAFE_INTEGER + 1])(
    "拒绝无效总数 %s",
    (total) => {
      expect(() => decodeListing({ ...page, total })).toThrow();
    },
  );
});
