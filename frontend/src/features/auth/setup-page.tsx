import {
  Button,
  Card,
  Form,
  Label,
  ListBox,
  Select,
  Spinner,
} from "@heroui/react";
import { Clock3, Folder, Globe2, Info, Play, UserRound } from "lucide-react";
import type { SyntheticEvent } from "react";
import { useState } from "react";

import mediaFolder from "@/assets/media-folder.svg";
import { ApiError } from "@/lib/api/errors";

import { AuthError } from "./auth-error";
import { AuthField } from "./auth-field";
import { useAuth } from "./auth-provider";
import {
  passwordHint,
  textValue,
  validateOutputDirectory,
  validatePassword,
  validateServiceUrl,
  validateUsername,
} from "./validation";

export function SetupPage() {
  const auth = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirmationError, setConfirmationError] = useState<
    Record<string, string>
  >({});
  async function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const values = new FormData(event.currentTarget);
    const password = textValue(values, "password");
    if (password !== textValue(values, "password_confirmation")) {
      setConfirmationError({ password_confirmation: "两次输入的密码不一致。" });
      return;
    }
    setConfirmationError({});
    setPending(true);
    setError(null);
    const lifetime = textValue(values, "session_lifetime_days");
    try {
      await auth.initialize({
        username: textValue(values, "username"),
        password,
        service_url: textValue(values, "service_url"),
        output_directory: textValue(values, "output_directory"),
        session_lifetime_days:
          lifetime === "1" ? 1 : lifetime === "30" ? 30 : 7,
      });
    } catch (error) {
      setError(error);
    } finally {
      setPending(false);
    }
  }
  return (
    <main className="auth-scene setup-scene">
      <div className="setup-layout">
        <section className="setup-banner" aria-labelledby="setup-title">
          <div>
            <h1 id="setup-title">首次使用 · 系统初始化</h1>
            <p>创建管理员并完成基础配置后，即可开始使用 115 Media Hub。</p>
          </div>
          <img src={mediaFolder} alt="" />
        </section>
        <Form
          className="setup-form"
          onSubmit={(event) => {
            void submit(event);
          }}
          validationErrors={{
            ...(error instanceof ApiError ? (error.fields ?? {}) : {}),
            ...confirmationError,
          }}
        >
          <div className="setup-grid">
            <Card className="setup-admin auth-section-card">
              <Card.Header className="setup-section-heading">
                <UserRound aria-hidden="true" />
                <div>
                  <h2>创建管理员账号</h2>
                  <p>用于登录系统并管理全部资源与配置</p>
                </div>
              </Card.Header>
              <Card.Content className="setup-fields">
                <AuthField
                  name="username"
                  label="管理员账号"
                  autoComplete="username"
                  placeholder="请输入管理员账号（如：admin）"
                  icon={<UserRound aria-hidden="true" />}
                  validate={validateUsername}
                  isDisabled={pending}
                />
                <AuthField
                  name="password"
                  label="设置密码"
                  description={passwordHint}
                  autoComplete="new-password"
                  type="password"
                  placeholder="请输入登录密码"
                  validate={validatePassword}
                  isDisabled={pending}
                />
                <AuthField
                  name="password_confirmation"
                  label="确认密码"
                  validate={validatePassword}
                  autoComplete="new-password"
                  type="password"
                  placeholder="请再次输入密码"
                  isDisabled={pending}
                />
                <Select
                  className="auth-field session-select"
                  name="session_lifetime_days"
                  defaultValue="7"
                  isRequired
                  isDisabled={pending}
                >
                  <Label>登录有效期</Label>
                  <Select.Trigger className="auth-input">
                    <Clock3 aria-hidden="true" />
                    <Select.Value />
                    <Select.Indicator />
                  </Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      {[1, 7, 30].map((days) => (
                        <ListBox.Item
                          key={days}
                          id={String(days)}
                          textValue={`${days} 天`}
                        >
                          {days} 天<ListBox.ItemIndicator />
                        </ListBox.Item>
                      ))}
                    </ListBox>
                  </Select.Popover>
                </Select>
              </Card.Content>
            </Card>
            <div className="setup-settings">
              <Card className="auth-section-card">
                <Card.Header className="setup-section-heading">
                  <Globe2 aria-hidden="true" />
                  <div>
                    <h2>服务访问地址</h2>
                    <p>用于对外访问本系统的地址（如反向代理后的地址）</p>
                  </div>
                </Card.Header>
                <Card.Content>
                  <AuthField
                    name="service_url"
                    label="服务地址"
                    type="url"
                    defaultValue={window.location.origin}
                    icon={<Globe2 aria-hidden="true" />}
                    validate={validateServiceUrl}
                    isDisabled={pending}
                    description="请输入 HTTP(S) 地址，不包含子路径。"
                  />
                </Card.Content>
              </Card>
              <Card className="auth-section-card">
                <Card.Header className="setup-section-heading">
                  <Folder aria-hidden="true" />
                  <div>
                    <h2>默认输出目录</h2>
                    <p>生成的 STRM 文件、NFO 等默认保存位置</p>
                  </div>
                </Card.Header>
                <Card.Content>
                  <AuthField
                    name="output_directory"
                    label="输出目录"
                    defaultValue="/app/media"
                    icon={<Folder aria-hidden="true" />}
                    validate={validateOutputDirectory}
                    isDisabled={pending}
                    description="服务器上的绝对路径；后续使用时需确保目录可写。"
                  />
                </Card.Content>
              </Card>
            </div>
          </div>
          <AuthError error={error} />
          <div className="setup-footer">
            <aside className="setup-notice">
              <Info aria-hidden="true" />
              <div>
                <h2>初始化说明</h2>
                <p>
                  该流程仅在首次部署时进行，用于创建管理员并配置基础运行参数。
                  <br className="desktop-break" />
                  完成后将自动进入系统首页。
                </p>
              </div>
            </aside>
            <Button
              className="auth-primary setup-submit"
              type="submit"
              isPending={pending}
            >
              {pending ? (
                <Spinner color="current" size="sm" />
              ) : (
                <Play aria-hidden="true" />
              )}
              {pending ? "正在初始化…" : "开始初始化"}
            </Button>
          </div>
        </Form>
      </div>
    </main>
  );
}
