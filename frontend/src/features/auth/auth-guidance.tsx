import { Clock3, Link2, ShieldCheck } from "lucide-react";

const guidance = [
  {
    icon: ShieldCheck,
    title: "安全提示",
    content: "请妥善保管您的账号与密码，避免在公共设备上登录。",
  },
  {
    icon: Clock3,
    title: "会话与重新登录",
    content:
      "会话按系统设定的有效期到期。到期或密码重置后，请重新登录继续使用。",
  },
  {
    icon: Link2,
    title: "页面跳转说明",
    content: "登录成功后将自动返回您登录前访问的页面，保留原页面的跳转目标。",
  },
];

export function AuthGuidance() {
  return (
    <aside className="auth-guidance" aria-label="登录说明">
      {guidance.map(({ icon: Icon, title, content }) => (
        <div className="guidance-row" key={title}>
          <Icon aria-hidden="true" />
          <div>
            <h2>{title}</h2>
            <p>{content}</p>
          </div>
        </div>
      ))}
    </aside>
  );
}
