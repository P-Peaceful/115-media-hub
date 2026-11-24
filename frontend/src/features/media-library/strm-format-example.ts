import type { LibraryInput } from "./api";

const defaultOrigin = "https://media.example.com";

/** Mirrors the 302 route with placeholder values instead of live playback credentials. */
export function formatSTRMExample(
  address: string,
  format: LibraryInput["strm_link_format"],
): string {
  const path = format === "file_id" ? "123456789" : "Movie%20Name.mkv";
  return `${exampleOrigin(address)}/p/artifact-id/playback-token/${path}`;
}

function exampleOrigin(address: string): string {
  const hasScheme = address.includes("://");
  if (address.length > 253 || (!hasScheme && address.endsWith("/")))
    return defaultOrigin;
  const origin = hasScheme ? address : `https://${address}`;
  const parts = /^(https?):\/\/(\[[0-9a-f:.]+\]|[a-z0-9.-]+)(:\d+)?\/?$/i.exec(
    origin,
  );
  if (!parts?.[1] || !parts[2] || parts[0] !== origin) return defaultOrigin;
  const [, scheme, host, port = ""] = parts;
  if (port && Number(port.slice(1)) > 65535) return defaultOrigin;
  if (!isPlaybackHost(host)) return defaultOrigin;
  return `${scheme.toLowerCase()}://${host}${port}`;
}

function isPlaybackHost(host: string): boolean {
  if (!host.startsWith("[")) {
    return host
      .split(".")
      .every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(label));
  }
  try {
    new URL(`https://${host}`);
    return true;
  } catch {
    return false;
  }
}
