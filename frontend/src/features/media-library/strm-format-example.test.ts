import { describe, expect, it } from "vitest";

import { formatSTRMExample } from "./strm-format-example";

describe("STRM illustrative format", () => {
  it("preserves valid host/port and IPv6 while changing placeholder paths", () => {
    expect(formatSTRMExample("[2001:db8::1]:8443", "file_id")).toBe(
      "https://[2001:db8::1]:8443/123456789",
    );
    expect(formatSTRMExample("media.local:443", "original_name")).toBe(
      "https://media.local:443/电影名称.mkv",
    );
  });
  it.each([
    "",
    "https://example.com/a",
    "media.local:99999",
    "media.local:",
    "a@media.local",
    "a b",
    "-invalid.com",
    "media.local?x=1",
  ])("uses the example domain for an empty/invalid draft: %s", (domain) => {
    expect(formatSTRMExample(domain, "file_id")).toBe(
      "https://media.example.com/123456789",
    );
  });
});
