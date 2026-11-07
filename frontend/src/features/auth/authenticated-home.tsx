import { Button } from "@heroui/react";
import { LogOut } from "lucide-react";
import { useState } from "react";

import { HealthPage } from "@/features/health/health-page";

import { AuthError } from "./auth-error";
import { useAuth } from "./auth-provider";

export function AuthenticatedHome() {
  const auth = useAuth();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function signOut() {
    setPending(true);
    setError(null);
    try {
      await auth.signOut();
    } catch (error) {
      setError(error);
    } finally {
      setPending(false);
    }
  }
  return (
    <HealthPage
      actions={
        <div className="home-auth-actions">
          <Button
            variant="secondary"
            isPending={pending}
            onPress={() => {
              void signOut();
            }}
          >
            <LogOut aria-hidden="true" className="size-4" />
            {pending ? "正在退出…" : "退出登录"}
          </Button>
          <AuthError error={error} />
        </div>
      }
    />
  );
}
