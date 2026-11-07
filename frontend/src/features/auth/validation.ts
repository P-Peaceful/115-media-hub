export function validateUsername(value: string) {
  return /^[A-Za-z0-9_.-]{3,32}$/.test(value)
    ? null
    : "使用 3–32 位英文字母、数字、下划线、点或连字符。";
}

export const passwordMaxLength = 32;
export const passwordHint = "8–32 位英文、数字或常用符号";

export function validatePassword(value: string) {
  return value.length >= 8 &&
    value.length <= passwordMaxLength &&
    !/[^!-~]/.test(value)
    ? null
    : "密码需为 8–32 位英文字母、数字或常用符号";
}

export function validateServiceUrl(value: string) {
  try {
    const url = new URL(value);
    const authority = value.split("/")[2] ?? "";
    if (
      new TextEncoder().encode(value).length <= 2048 &&
      !/[\p{Cc}\s\\]/u.test(value) &&
      !authority.includes("@") &&
      value === value.trim() &&
      /^https?:\/\//i.test(value) &&
      !value.includes("?") &&
      !value.includes("#") &&
      !authority.endsWith(":") &&
      (!url.port || Number(url.port) >= 1) &&
      ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash &&
      url.pathname === "/"
    )
      return null;
  } catch {
    /* Invalid URLs share the same actionable message. */
  }
  return "请输入完整的 HTTP(S) 服务地址，不含账号、路径、查询或片段。";
}

export function validateOutputDirectory(value: string) {
  return new TextEncoder().encode(value).length <= 4096 &&
    value === value.trim() &&
    value.startsWith("/") &&
    !/\p{Cc}/u.test(value) &&
    !value.includes("\\") &&
    !value.split("/").some((part) => part === "." || part === "..") &&
    !value.includes("//") &&
    (value === "/" || !value.endsWith("/"))
    ? null
    : "请输入规范的服务器绝对路径，例如 /app/media。";
}

export function textValue(form: FormData, name: string): string {
  const value = form.get(name);
  return typeof value === "string" ? value : "";
}
