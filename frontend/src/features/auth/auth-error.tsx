import { Alert } from "@heroui/react";

import { ApiError } from "@/lib/api/errors";

export function AuthError({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <Alert status="danger" role="alert" className="auth-error">
      <Alert.Indicator />
      <Alert.Content>
        <Alert.Title>
          {error instanceof ApiError
            ? error.message
            : "操作未完成，请稍后重试。"}
        </Alert.Title>
        {error instanceof ApiError && error.requestId && (
          <Alert.Description>请求编号：{error.requestId}</Alert.Description>
        )}
      </Alert.Content>
    </Alert>
  );
}
