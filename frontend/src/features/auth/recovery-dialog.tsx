import { Button, Link, Modal } from "@heroui/react";
import { Terminal } from "lucide-react";

export function RecoveryDialog() {
  return (
    <Modal>
      <Button className="forgot-password" variant="ghost">
        忘记密码？
      </Button>
      <Modal.Backdrop variant="blur">
        <Modal.Container>
          <Modal.Dialog className="recovery-dialog">
            <Modal.CloseTrigger aria-label="关闭" />
            <Modal.Header>
              <Modal.Icon>
                <Terminal aria-hidden="true" />
              </Modal.Icon>
              <Modal.Heading>重置管理员密码</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <p>请在部署服务器上停止服务，使用相同的数据目录执行：</p>
              <pre>
                <code>mediahub admin reset-password</code>
              </pre>
              <p>
                按提示输入新密码并确认，然后重启服务。新密码不会显示在终端中。重置后全部旧会话失效，系统配置会保留。
              </p>
              <p>
                容器部署需挂载原数据卷，使用交互终端或显式使用{" "}
                <code>--password-stdin</code> 输入模式。
              </p>
              <Link
                href="https://github.com/wenzhengcheng/115-media-hub#管理员密码恢复"
                target="_blank"
                rel="noopener noreferrer"
              >
                查看部署与恢复说明
                <Link.Icon />
              </Link>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" className="auth-primary">
                我知道了
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
