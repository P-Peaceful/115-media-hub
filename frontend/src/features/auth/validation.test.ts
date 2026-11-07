import { describe, expect, it } from "vitest";

import { safeReturnPath } from "./return-path";
import {
  validateOutputDirectory,
  validatePassword,
  validateServiceUrl,
  validateUsername,
} from "./validation";

describe("认证输入", () => {
  it.each([
    "https://evil.example",
    "//evil.example",
    "/\\evil.example",
    "/%5cevil.example",
    "/login",
    "/%6cogin",
    "/setup/next",
    "/\n/evil.example",
    "/%00bad",
    "%",
  ])("阻止不安全的返回地址 %s", (value) => {
    expect(safeReturnPath(value)).toBe("/");
  });
  it("保留本地路径查询和片段", () => {
    expect(safeReturnPath("/libraries?tab=recent#files")).toBe(
      "/libraries?tab=recent#files",
    );
  });
  it.each([
    "a".repeat(8),
    "Z".repeat(32),
    "12345678",
    "!@#$%^&*()_+-=[]{};:'\",.<>/?|~`\\",
  ])("接受边界长度和常规ASCII密码 %#", (password) => {
    expect(validatePassword(password)).toBeNull();
  });
  it.each([
    "",
    "a".repeat(7),
    "a".repeat(33),
    "中文password",
    "pássword",
    "password😀",
    " pass123",
    "pass123 ",
    "pass 123",
    "password\t",
    "password\n",
    "password\r",
    "password\r\n",
    "password\u0000",
    "password\u007f",
    "password\u0085",
  ])("拒绝非法密码且不修剪内容 %#", (password) => {
    expect(validatePassword(password)).not.toBeNull();
  });
  it("用户名保持ASCII规则", () => {
    expect(validateUsername("admin.user-1")).toBeNull();
    expect(validateUsername("管理者")).not.toBeNull();
  });
  it.each([
    "https://example.com/prefix",
    "https://user:pass@example.com",
    "ftp://example.com",
    "https://example.com?token=bad",
    "https://example.com#bad",
    "https://example.com#",
    "https://example.com?",
    " https://example.com",
    "https://example.com:0",
    "https://example.com:",
    "https:example.com",
    "https://exa\tmple.com",
    "https://example.com\\",
    "https://@example.com",
  ])("拒绝不支持的服务URL %s", (value) => {
    expect(validateServiceUrl(value)).not.toBeNull();
  });
  it("接受地址源和服务器规范绝对路径", () => {
    expect(validateServiceUrl("https://media.example.com/")).toBeNull();
    expect(validateOutputDirectory("/media")).toBeNull();
    expect(validateOutputDirectory("../media")).not.toBeNull();
    expect(validateOutputDirectory("/media ")).not.toBeNull();
    expect(validateOutputDirectory("/media\u0085")).not.toBeNull();
    expect(validateOutputDirectory("/" + "a".repeat(4096))).not.toBeNull();
    expect(validateOutputDirectory("/data/../media")).not.toBeNull();
  });
});
