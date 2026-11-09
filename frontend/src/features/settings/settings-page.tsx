import {
  Button,
  Card,
  Form,
  Input,
  InputGroup,
  Label,
  ListBox,
  Select,
  TextField,
} from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  Eye,
  EyeOff,
  KeyRound,
  LogOut,
  QrCode,
  RefreshCw,
  Settings,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import type { SyntheticEvent } from "react";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";

import { AuthError } from "@/features/auth/auth-error";
import { useAuth } from "@/features/auth/auth-provider";
import {
  textValue,
  validatePassword,
  validateUsername,
} from "@/features/auth/validation";

import { formatBytes, useAccountDetails } from "./account-details";
import * as api from "./api";

const securityKey = ["settings", "security"] as const;
const accountKey = ["settings", "115"] as const;

function SectionHeading({
  icon: Icon,
  title,
  description,
}: {
  icon: typeof UserRound;
  title: string;
  description: string;
}) {
  return (
    <div className="settings-section-heading">
      <span className="settings-icon">
        <Icon aria-hidden="true" />
      </span>
      <div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
    </div>
  );
}

function PasswordField({
  name,
  label,
  placeholder,
  autoComplete,
  required = false,
}: {
  name: string;
  label: string;
  placeholder: string;
  autoComplete: string;
  required?: boolean;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <TextField name={name} isRequired={required}>
      <Label>{label}</Label>
      <InputGroup>
        <InputGroup.Input
          type={visible ? "text" : "password"}
          autoComplete={autoComplete}
          placeholder={placeholder}
        />
        <InputGroup.Suffix>
          <Button
            type="button"
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={`${visible ? "隐藏" : "显示"}${label}`}
            aria-pressed={visible}
            onPress={() => setVisible(!visible)}
          >
            {visible ? (
              <Eye aria-hidden="true" />
            ) : (
              <EyeOff aria-hidden="true" />
            )}
          </Button>
        </InputGroup.Suffix>
      </InputGroup>
    </TextField>
  );
}

function SecuritySettings() {
  const auth = useAuth();
  const queryClient = useQueryClient();
  const csrf = auth.data?.csrf_token ?? "";
  const security = useQuery({
    queryKey: securityKey,
    queryFn: ({ signal }) => api.getSecurity(signal),
    retry: false,
  });
  const [formError, setFormError] = useState("");
  const [message, setMessage] = useState("");
  const credentials = useMutation({
    mutationFn: (body: {
      username: string;
      current_password: string;
      new_password: string;
    }) => api.updateCredentials(csrf, body),
    onSuccess: async (data) => {
      queryClient.setQueryData(securityKey, data);
      await auth.retry();
      setMessage("管理员账号已更新，其他会话已退出。");
    },
  });
  const session = useMutation({
    mutationFn: (days: 1 | 7 | 30) => api.updateSession(csrf, days),
    onSuccess: (data) => {
      queryClient.setQueryData(securityKey, data);
      setMessage("登录有效期已保存，将应用于后续登录。");
    },
  });
  const logout = useMutation({ mutationFn: () => auth.signOut() });
  function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const username = textValue(values, "username");
    const current = textValue(values, "current_password");
    const password = textValue(values, "new_password");
    const confirmation = textValue(values, "confirmation");
    const error =
      validateUsername(username) ??
      (!current ? "请输入当前密码。" : null) ??
      (password ? validatePassword(password) : null) ??
      (password !== confirmation ? "两次输入的新密码不一致。" : null);
    setFormError(error ?? "");
    setMessage("");
    if (!error)
      credentials.mutate(
        { username, current_password: current, new_password: password },
        { onSuccess: () => form.reset() },
      );
  }
  if (security.isPending)
    return <p className="settings-state">正在加载设置…</p>;
  if (security.isError)
    return (
      <div className="settings-state">
        <AuthError error={security.error} />
        <Button variant="secondary" onPress={() => void security.refetch()}>
          重试
        </Button>
      </div>
    );
  return (
    <div className="settings-cards">
      <Card className="settings-card">
        <Card.Content>
          <Form
            key={security.data.username}
            className="settings-form"
            onSubmit={submit}
          >
            <div className="settings-card-top">
              <SectionHeading
                icon={UserRound}
                title="管理员账号"
                description="修改系统管理员的用户名和密码。"
              />
              <Button
                className="settings-primary"
                type="submit"
                isPending={credentials.isPending}
              >
                保存修改
              </Button>
            </div>
            <div className="settings-fields">
              <TextField
                name="username"
                defaultValue={security.data.username}
                isRequired
              >
                <Label>用户名</Label>
                <Input />
              </TextField>
              <PasswordField
                name="current_password"
                label="当前密码"
                required
                autoComplete="current-password"
                placeholder="请输入当前密码"
              />
              <PasswordField
                name="new_password"
                label="新密码"
                autoComplete="new-password"
                placeholder="不修改密码可留空"
              />
              <PasswordField
                name="confirmation"
                label="确认新密码"
                autoComplete="new-password"
                placeholder="请再次输入新密码"
              />
            </div>
            {formError && (
              <p className="settings-error" role="alert">
                {formError}
              </p>
            )}
            <AuthError error={credentials.error} />
          </Form>
        </Card.Content>
      </Card>
      <Card className="settings-card">
        <Card.Content>
          <div className="settings-card-top">
            <SectionHeading
              icon={KeyRound}
              title="登录安全"
              description="配置登录会话的有效期。"
            />
          </div>
          <div className="settings-fields">
            <Select
              name="session_lifetime_days"
              selectedKey={String(security.data.session_lifetime_days)}
              onSelectionChange={(key) => {
                if (key) {
                  setMessage("");
                  if (key === "1" || key === "7" || key === "30")
                    session.mutate(key === "1" ? 1 : key === "30" ? 30 : 7);
                }
              }}
              isDisabled={session.isPending}
            >
              <Label>登录有效期</Label>
              <Select.Trigger>
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
            <p className="settings-help">
              登录后，在该时间内保持会话有效，超过后需重新登录。已创建的会话不受影响。
            </p>
            <p className="settings-help">是否记住登录状态仍可在登录时选择。</p>
          </div>
          <AuthError error={session.error} />
        </Card.Content>
      </Card>
      <Card className="settings-card">
        <Card.Content>
          <div className="settings-card-top">
            <SectionHeading
              icon={LogOut}
              title="会话管理"
              description="退出当前会话，将清除当前的登录状态。"
            />
            <Button
              variant="danger-soft"
              isPending={logout.isPending}
              onPress={() => logout.mutate()}
            >
              退出当前会话
            </Button>
          </div>
          <AuthError error={logout.error} />
        </Card.Content>
      </Card>
      {message && (
        <p className="settings-success" role="status">
          {message}
        </p>
      )}
    </div>
  );
}

function AccountSettings() {
  const auth = useAuth();
  const csrf = auth.data?.csrf_token ?? "";
  const queryClient = useQueryClient();
  const [qrId, setQrId] = useState<string | null>(null);
  const details = useAccountDetails();
  const account = useQuery({
    queryKey: accountKey,
    queryFn: ({ signal }) => api.getAccount(signal),
    retry: false,
  });
  const start = useMutation({
    mutationFn: () => api.createQr(csrf),
    onSuccess: (qr) => {
      queryClient.setQueryData(["settings", "qr", qr.id], qr);
      setQrId(qr.id);
    },
  });
  const qr = useQuery({
    queryKey: ["settings", "qr", qrId],
    queryFn: ({ signal }) => api.getQr(qrId ?? "", csrf, signal),
    enabled: qrId !== null,
    retry: false,
    refetchInterval: (query) =>
      ["waiting", "scanned"].includes(query.state.data?.status ?? "")
        ? 2000
        : false,
  });
  const cancel = useMutation({
    mutationFn: (id: string) => api.cancelQr(csrf, id),
    onSuccess: () => setQrId(null),
  });
  const remove = useMutation({
    mutationFn: () => api.removeAccount(csrf),
    onSuccess: async () => {
      setQrId(null);
      await queryClient.invalidateQueries({ queryKey: accountKey });
    },
  });
  const succeeded = qr.data?.status === "succeeded";
  // Authorization can be completed without pressing the confirmation button.
  useEffect(() => {
    if (succeeded) void queryClient.invalidateQueries({ queryKey: accountKey });
  }, [queryClient, succeeded]);
  const visibleAccount = succeeded ? null : account.data;
  function finish() {
    setQrId(null);
  }
  const statuses: Record<string, string> = {
    waiting: "等待扫码",
    scanned: "已扫码，请在 115 App 中确认",
    succeeded: "授权成功",
    expired: "二维码已过期",
    canceled: "扫码已取消",
  };
  return (
    <div className="settings-cards">
      <Card className="settings-card">
        <Card.Content>
          <div className="settings-card-top">
            <SectionHeading
              icon={Cloud}
              title="115 账号"
              description="通过 115 App 扫码授权，连接你的网盘账号。"
            />
          </div>
          {account.isPending && (
            <p className="settings-state">正在加载账号信息…</p>
          )}
          {account.isError && (
            <>
              <AuthError error={account.error} />
              <Button
                variant="secondary"
                onPress={() => void account.refetch()}
              >
                重试
              </Button>
            </>
          )}
          {visibleAccount && (
            <div className="account-summary">
              <span
                className={
                  visibleAccount.connected &&
                  details.data?.status === "valid" &&
                  !details.isError
                    ? "account-status connected"
                    : "account-status"
                }
              >
                {!visibleAccount.connected
                  ? "未授权"
                  : details.isError
                    ? "状态未知"
                    : details.isPending
                      ? "正在验证"
                      : details.data.status === "valid"
                        ? "授权有效"
                        : details.data.status === "invalid"
                          ? "授权失效"
                          : "未授权"}
              </span>
              <p>
                {visibleAccount.connected
                  ? visibleAccount.account_name || "本地已保存 115 授权"
                  : "尚未配置 115 账号，请扫码登录。"}
              </p>
              {visibleAccount.connected_at && (
                <p>
                  授权时间：
                  {new Date(visibleAccount.connected_at).toLocaleString()}
                </p>
              )}
              {visibleAccount.connected && (
                <Button
                  variant="secondary"
                  isPending={details.isFetching}
                  onPress={() => void details.refetch({ cancelRefetch: true })}
                >
                  <RefreshCw aria-hidden="true" className="size-4" />
                  {details.isFetching ? "正在检查授权…" : "检查授权状态"}
                </Button>
              )}
            </div>
          )}
          {visibleAccount?.connected && (
            <div className="account-details" aria-live="polite">
              {details.isPending ? (
                <p>正在获取 115 账号资料…</p>
              ) : details.isError ? (
                <>
                  <p role="status">
                    无法确认授权状态，检查请求失败，请检查网络后重试。
                  </p>
                  <Button
                    variant="secondary"
                    isPending={details.isFetching}
                    onPress={() =>
                      void details.refetch({ cancelRefetch: true })
                    }
                  >
                    重新检查
                  </Button>
                </>
              ) : details.data.status === "invalid" ? (
                <p>授权已失效，请重新扫码授权。</p>
              ) : details.data.status === "valid" && details.data.profile ? (
                <>
                  <div className="account-profile">
                    {details.data.profile.avatar && (
                      <img
                        className="account-avatar"
                        src={details.data.profile.avatar}
                        alt={`${details.data.profile.user_name}头像`}
                        referrerPolicy="no-referrer"
                      />
                    )}
                    <strong>
                      {details.data.profile.user_name || "115 用户"}
                    </strong>
                    <span>
                      {details.data.profile.is_member === null
                        ? "会员状态未知"
                        : details.data.profile.is_member
                          ? "115 会员"
                          : "非会员"}
                    </span>
                  </div>
                  <dl>
                    <div>
                      <dt>昵称</dt>
                      <dd>{details.data.profile.user_name || "暂无数据"}</dd>
                    </div>
                    <div>
                      <dt>用户 ID</dt>
                      <dd>{details.data.profile.user_id ?? "暂无数据"}</dd>
                    </div>
                    <div>
                      <dt>会员有效期</dt>
                      <dd>
                        {details.data.profile.member_forever === true
                          ? "永久会员"
                          : details.data.profile.member_expires_at
                            ? new Date(
                                details.data.profile.member_expires_at,
                              ).toLocaleString()
                            : details.data.profile.is_member === false
                              ? "无会员有效期"
                              : "暂无数据"}
                      </dd>
                    </div>
                    <div>
                      <dt>网盘容量</dt>
                      <dd>
                        {details.data.space_status === "available" &&
                        details.data.space
                          ? `${formatBytes(details.data.space.used_bytes)} / ${formatBytes(details.data.space.total_bytes)}（已用 / 总容量）`
                          : "暂不可用"}
                      </dd>
                    </div>
                  </dl>
                  <section className="account-devices" aria-label="登录设备">
                    <h3>登录设备</h3>
                    {details.data.devices_status !== "available" ? (
                      <p>设备信息暂不可用。</p>
                    ) : details.data.devices.length === 0 ? (
                      <p>暂无设备信息。</p>
                    ) : (
                      <ul>
                        {details.data.devices.map((device, index) => (
                          <li key={`${device.name}-${device.ip}-${index}`}>
                            <strong>
                              {device.name || device.type || "未知设备"}
                            </strong>
                            {device.is_current && <span>当前设备</span>}
                            <span>
                              {[device.type, device.ip, device.city]
                                .filter(Boolean)
                                .join(" · ") || "暂无详细信息"}
                            </span>
                            <time>
                              {device.login_at
                                ? `最近登录：${new Date(device.login_at).toLocaleString()}`
                                : "登录时间未知"}
                            </time>
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                </>
              ) : (
                <p>尚未配置 115 授权。</p>
              )}
            </div>
          )}
          {qrId && (
            <div className="qr-panel" aria-live="polite">
              {qr.data && ["waiting", "scanned"].includes(qr.data.status) && (
                <div className="qr-image">
                  <img
                    src={qr.data.qr_url}
                    alt="115 登录二维码"
                    width={196}
                    height={196}
                  />
                </div>
              )}
              <p>
                {qr.isPending
                  ? "正在获取二维码…"
                  : qr.data
                    ? statuses[qr.data.status]
                    : "扫码状态获取失败"}
              </p>
              <AuthError error={qr.error} />
              <div className="settings-actions">
                {succeeded ? (
                  <Button className="settings-primary" onPress={finish}>
                    完成
                  </Button>
                ) : qr.isError ||
                  (qr.data &&
                    ["expired", "canceled"].includes(qr.data.status)) ? (
                  <Button
                    className="settings-primary"
                    onPress={() => {
                      setQrId(null);
                      start.mutate();
                    }}
                  >
                    重新获取二维码
                  </Button>
                ) : (
                  <Button
                    variant="secondary"
                    isPending={cancel.isPending}
                    onPress={() => cancel.mutate(qrId)}
                  >
                    取消扫码
                  </Button>
                )}
              </div>
              <AuthError error={cancel.error} />
            </div>
          )}
          {!qrId && account.data && (
            <div className="settings-actions">
              <Button
                className="settings-primary"
                isPending={start.isPending}
                onPress={() => start.mutate()}
              >
                <QrCode aria-hidden="true" />
                {account.data.connected ? "重新扫码授权" : "扫码登录"}
              </Button>
              {account.data.connected && (
                <Button
                  variant="danger-soft"
                  isPending={remove.isPending}
                  onPress={() => remove.mutate()}
                >
                  移除授权
                </Button>
              )}
            </div>
          )}
          <AuthError error={start.error} />
          <AuthError error={remove.error} />
        </Card.Content>
      </Card>
    </div>
  );
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = searchParams.get("tab") === "account" ? "account" : "security";
  function setTab(next: "security" | "account") {
    setSearchParams(next === "account" ? { tab: "account" } : {});
  }
  return (
    <main className="settings-page">
      <section className="settings-banner">
        <div>
          <h1>设置</h1>
          <p>
            管理系统配置，包括账号安全、115
            授权、服务访问、生成规则、元数据、性能等。
          </p>
        </div>
        <div className="settings-banner-art" aria-hidden="true">
          <Settings />
        </div>
      </section>
      <nav className="settings-tabs" aria-label="设置分类">
        <button
          className={tab === "security" ? "selected" : ""}
          aria-current={tab === "security" ? "page" : undefined}
          onClick={() => setTab("security")}
        >
          <ShieldCheck aria-hidden="true" />
          管理员与安全
        </button>
        <button
          className={tab === "account" ? "selected" : ""}
          aria-current={tab === "account" ? "page" : undefined}
          onClick={() => setTab("account")}
        >
          <Cloud aria-hidden="true" />
          115 账号
        </button>
      </nav>
      {tab === "security" ? <SecuritySettings /> : <AccountSettings />}
    </main>
  );
}
