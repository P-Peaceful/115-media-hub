import { describe, expect, it } from "vitest";

import { formatSTRMExample } from "./strm-format-example";

describe("STRM illustrative format", () => {
  it("preserves valid host/port and IPv6 while changing placeholder paths", () => {
    expect(formatSTRMExample("[2001:db8::1]:8443", "file_id")).toBe(
      "https://[2001:db8::1]:8443/p/artifact-id/playback-token/123456789",
    );
    expect(formatSTRMExample("media.local:443", "original_name")).toBe(
      "https://media.local:443/p/artifact-id/playback-token/Movie%20Name.mkv",
    );
    expect(formatSTRMExample("http://192.168.1.10:8080/", "file_id")).toBe(
      "http://192.168.1.10:8080/p/artifact-id/playback-token/123456789",
    );
    expect(formatSTRMExample("dev.123", "file_id")).toBe(
      "https://dev.123/p/artifact-id/playback-token/123456789",
    );
  });
  it.each([
    "",
    "https://example.com/a",
    "media.local/",
    "media.local:99999",
    "media.local:",
    "a@media.local",
    "a b",
    "-invalid.com",
    "media.local?x=1",
    "https://media.local/#",
    "ftp://media.local",
  ])("uses the example domain for an empty/invalid draft: %s", (domain) => {
    expect(formatSTRMExample(domain, "file_id")).toBe(
      "https://media.example.com/p/artifact-id/playback-token/123456789",
    );
  });
});
