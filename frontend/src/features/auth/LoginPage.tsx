import { Alert, Button, Chip, Form, Input, Label, Spinner, TextField } from "@heroui/react";
import { useState, type FormEvent } from "react";
import {
  ArrowRight,
  BriefcaseBusiness,
  Check,
  FileText,
  FolderOpen,
  LibraryBig,
} from "lucide-react";
import { Navigate, useLocation, useNavigate } from "react-router";
import { useAuth } from "./auth-context";
import { safeReturnTo } from "./auth-state";
import "./auth-login.css";

export function LoginPage() {
  const { status, signIn, reload } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const from = safeReturnTo(
    (location.state as { from?: unknown } | null)?.from,
  );

  if (status === "loading")
    return (
      <main className="auth-login auth-login--message" aria-live="polite">
        <div className="auth-login__loading">
          <Spinner aria-label="正在验证管理会话" />
          正在验证管理会话…
        </div>
      </main>
    );
  if (status === "error")
    return (
      <main className="auth-login auth-login--message">
        <section className="auth-login__card">
          <Alert status="danger" role="alert">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Description>无法连接服务，请重试。</Alert.Description>
            </Alert.Content>
          </Alert>
          <Button variant="primary" className="auth-login__submit" onPress={() => void reload()}>
            重试
          </Button>
        </section>
      </main>
    );
  if (status === "authenticated") return <Navigate to={from} replace />;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      await signIn(username, password);
      navigate(from, { replace: true });
    } catch {
      setError("账号或密码错误，请重试。");
    } finally {
      setPending(false);
      setPassword("");
    }
  }

  return (
    <main className="auth-login">
      <header className="auth-login__header">
        <span className="auth-login__icon">
          <BriefcaseBusiness size={17} aria-hidden="true" />
        </span>
        <span>115 Media Hub</span>
        {/*<span className="auth-login__header-label">媒体管理工作台</span>*/}
      </header>
      <div className="auth-login__layout">
        <section
          className="auth-login__marketing"
          aria-labelledby="marketing-title"
        >
          <h1 id="marketing-title" className="auth-login__marketing-title">
            把网盘影片，
            <br />
            整理成你的媒体库。
          </h1>
          <p className="auth-login__marketing-copy">
            逐级浏览目录，生成 STRM。每次变更先预览，后台执行有记录。
          </p>
          <div className="auth-login__flow" aria-label="从网盘目录到家庭媒体库">
            <div className="auth-login__flow-heading">
              <span>
                <FolderOpen size={14} aria-hidden="true" />
                从网盘目录到家庭媒体库
              </span>
              <Chip className="auth-login__flow-badge" variant="secondary" color="default" size="sm">
                <i aria-hidden="true" />
                流程示意
              </Chip>
            </div>
            <div className="auth-login__flow-content">
              <div className="auth-login__flow-path">
                115 网盘 / 媒体 / 电影
              </div>
              <div className="auth-login__source-items">
                <span>
                  <FileText size={14} aria-hidden="true" />
                  雾港来信 (2025)
                </span>
                <span>
                  <FileText size={14} aria-hidden="true" />
                  环海列车 (2024)
                </span>
              </div>
            </div>
            <div className="auth-login__flow-caption">
              <ArrowRight size={14} aria-hidden="true" />
              确认范围与差异，再生成 STRM
            </div>
            <div className="auth-login__library">
              <LibraryBig size={18} aria-hidden="true" />
              <div>
                <strong>电影库</strong>
                <small>关联 Emby / Jellyfin，按真实文件核对结果</small>
              </div>
            </div>
          </div>
          <div className="auth-login__chips">
            <Chip className="auth-login__chip" variant="secondary" color="default">
              <Check size={14} aria-hidden="true" />
              范围清楚
            </Chip>
            <Chip className="auth-login__chip" variant="secondary" color="default">
              <Check size={14} aria-hidden="true" />
              变更可预览
            </Chip>
            <Chip className="auth-login__chip" variant="secondary" color="default">
              <Check size={14} aria-hidden="true" />
              运行可追溯
            </Chip>
          </div>
        </section>
        <section className="auth-login__panel" aria-labelledby="login-title">
          <div className="auth-login__card">
            <h2 id="login-title" className="auth-login__title">
              欢迎回来
            </h2>
            <p className="auth-login__subtitle">
              登录管理界面，继续管理你的媒体库。
            </p>
            <Form className="auth-login__form" onSubmit={submit}>
              <TextField name="username" className="auth-login__field" isRequired fullWidth>
                <Label className="auth-login__label">管理员账号</Label>
                <Input
                  autoComplete="username"
                  placeholder="管理员"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  className="auth-login__input"
                />
              </TextField>
              <TextField name="password" className="auth-login__field" isRequired fullWidth>
                <Label className="auth-login__label">密码</Label>
                <Input
                  type="password"
                  autoComplete="current-password"
                  placeholder="••••••••••••"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="auth-login__input"
                />
              </TextField>
              {error && (
                <Alert status="danger" role="alert" className="auth-login__error">
                  <Alert.Content>
                    <Alert.Description>{error}</Alert.Description>
                  </Alert.Content>
                </Alert>
              )}
              <Button
                type="submit"
                variant="primary"
                isDisabled={pending}
                className="auth-login__submit"
              >
                <ArrowRight size={16} aria-hidden="true" />
                {pending ? "正在登录…" : "登录管理界面"}
              </Button>
            </Form>
            <div className="auth-login__help">
              <p>使用部署时设置的管理员凭据</p>
              <p>此处登录用于管理操作。115 网盘授权将在登录后单独配置。</p>
            </div>
          </div>
          <p className="auth-login__return-note">
            登录成功后返回原访问页面。会话过期时保留非敏感的筛选与定位信息。
          </p>
        </section>
      </div>
      <footer className="auth-login__footer">
        你决定处理范围，工具记录每一次变更。
      </footer>
    </main>
  );
}
