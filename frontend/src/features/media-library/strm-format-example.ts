import type { LibraryInput } from "./api";

/** Presentation-only placeholder paths; playback routes are not implemented yet. */
export function formatSTRMExample(
  domain: string,
  format: LibraryInput["strm_link_format"],
): string {
  const host = domain.trim() || "media.example.com";
  const path = format === "file_id" ? "123456789" : "电影名称.mkv";
  // Keep malformed drafts as a fallback example, never interpret input as a URL.
  let exampleHost = "media.example.com";
  try {
    const parsed = new URL(`https://${host}`);
    if (
      parsed.host &&
      parsed.hostname &&
      !/[\s@\\]/.test(host) &&
      (parsed.hostname.startsWith("[") ||
        parsed.hostname
          .split(".")
          .every((label) =>
            /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(label),
          )) &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.pathname === "/" &&
      parsed.search === "" &&
      parsed.hash === "" &&
      !host.includes("/") &&
      !host.endsWith(":")
    ) {
      exampleHost = host;
    }
  } catch {
    // An incomplete host/port is normal while typing.
  }
  return `https://${exampleHost}/${path}`;
}
