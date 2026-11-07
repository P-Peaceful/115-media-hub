import { Button, Card, Checkbox, Form, Spinner } from "@heroui/react";
import { UserRound } from "lucide-react";
import type { SyntheticEvent } from "react";
import { useState } from "react";

import mediaFolder from "@/assets/media-folder.svg";
import { ApiError } from "@/lib/api/errors";

import { AuthError } from "./auth-error";
import { AuthField } from "./auth-field";
import { AuthGuidance } from "./auth-guidance";
import { useAuth } from "./auth-provider";
import { RecoveryDialog } from "./recovery-dialog";
import { textValue, validatePassword, validateUsername } from "./validation";

export function LoginPage() {
  const auth = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [remember, setRemember] = useState(true);
  async function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const values = new FormData(event.currentTarget);
    setError(null);
    setPending(true);
    try {
      await auth.signIn({
        username: textValue(values, "username"),
        password: textValue(values, "password"),
        remember_login: remember,
      });
    } catch (error) {
      setError(error);
    } finally {
      setPending(false);
    }
  }
  return (
    <main className="auth-scene login-scene">
      <div className="auth-wave" aria-hidden="true" />
      <div className="login-layout">
        <section className="login-brand" aria-labelledby="brand-name">
          <h1 id="brand-name">115 Media Hub</h1>
          <p>基于 115 网盘的媒体管理与 STRM 生态解决方案</p>
          <img src={mediaFolder} alt="" className="login-illustration" />
        </section>
        <Card className="login-card">
          <Card.Header className="login-card-heading">
            <span className="auth-heading-icon">
              <UserRound aria-hidden="true" />
            </span>
            <div>
              <h2>管理员登录</h2>
              <p>登录后可管理媒体资源、任务与系统配置</p>
            </div>
          </Card.Header>
          <Card.Content>
            <Form
              className="login-form"
              onSubmit={(event) => {
                void submit(event);
              }}
              validationErrors={
                error instanceof ApiError ? (error.fields ?? {}) : {}
              }
            >
              <AuthField
                name="username"
                validate={validateUsername}
                label="用户名"
                autoComplete="username"
                placeholder="请输入管理员用户名"
                icon={<UserRound aria-hidden="true" />}
                isDisabled={pending}
              />
              <AuthField
                name="password"
                validate={validatePassword}
                label="密码"
                autoComplete="current-password"
                placeholder="请输入登录密码"
                type="password"
                isDisabled={pending}
              />
              <div className="login-options">
                <Checkbox
                  className="remember-checkbox"
                  isSelected={remember}
                  onChange={setRemember}
                  isDisabled={pending}
                >
                  <Checkbox.Content>
                    <Checkbox.Control>
                      <Checkbox.Indicator />
                    </Checkbox.Control>
                    记住登录状态
                  </Checkbox.Content>
                </Checkbox>
                <RecoveryDialog />
              </div>
              <AuthError error={error} />
              <Button
                className="auth-primary login-submit"
                type="submit"
                isPending={pending}
              >
                {pending && <Spinner color="current" size="sm" />}
                {pending ? "正在登录…" : "登录"}
              </Button>
            </Form>
            <AuthGuidance />
          </Card.Content>
        </Card>
      </div>
    </main>
  );
}
