import { Button, Form } from "@heroui/react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AuthField } from "./auth-field";
import { passwordHint, validatePassword } from "./validation";

function renderPasswordForm() {
  const submit = vi.fn();
  render(
    <Form
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <AuthField
        name="password"
        label="密码"
        type="password"
        validate={validatePassword}
        description={passwordHint}
      />
      <Button type="submit">提交</Button>
    </Form>,
  );
  return submit;
}

describe("密码字段", () => {
  it("保留原始超长粘贴值并拒绝提交，不截断为另一密码", async () => {
    const user = userEvent.setup();
    const submit = renderPasswordForm();
    const input = screen.getByLabelText("密码");
    const password = "a".repeat(33);
    await user.click(input);
    await user.paste(password);
    expect(input).toHaveValue(password);
    await user.click(screen.getByRole("button", { name: "提交" }));
    expect(submit).not.toHaveBeenCalled();
    expect(
      await screen.findByText("密码需为 8–32 位英文字母、数字或常用符号"),
    ).toBeVisible();
    expect(input).toHaveAttribute("aria-invalid", "true");
  });
  it.each(["中文password", "password "])(
    "拒绝非法字符且保留原始内容 %#",
    async (password) => {
      const user = userEvent.setup();
      const submit = renderPasswordForm();
      const input = screen.getByLabelText("密码");
      await user.click(input);
      await user.paste(password);
      await user.click(screen.getByRole("button", { name: "提交" }));
      expect(input).toHaveValue(password);
      expect(submit).not.toHaveBeenCalled();
      expect(input).toHaveAttribute("aria-invalid", "true");
    },
  );
  it("接受32位密码且显示明确规则，不强制字符组合", async () => {
    const user = userEvent.setup();
    const submit = renderPasswordForm();
    const input = screen.getByLabelText("密码");
    await user.click(input);
    await user.paste("1".repeat(32));
    expect(screen.getByText(passwordHint)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "提交" }));
    expect(submit).toHaveBeenCalledOnce();
  });
});
