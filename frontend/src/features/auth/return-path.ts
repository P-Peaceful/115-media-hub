export function safeReturnPath(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//")) return "/";
  try {
    const decoded = decodeURIComponent(value);
    if (/[\\\s\p{Cc}]/u.test(decoded) || decoded.startsWith("//")) return "/";
    const target = new URL(value, "https://mediahub.invalid");
    if (
      target.origin !== "https://mediahub.invalid" ||
      /^\/(?:login|setup)(?:\/|$)/i.test(decodeURIComponent(target.pathname))
    )
      return "/";
    return `${target.pathname}${target.search}${target.hash}`;
  } catch {
    return "/";
  }
}
