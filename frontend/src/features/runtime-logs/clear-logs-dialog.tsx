import {
  Alert,
  AlertDialog,
  Button,
  Description,
  Label,
  Radio,
  RadioGroup,
} from "@heroui/react";
import { useState } from "react";

import { ApiError } from "@/lib/api/errors";

import type { ClearRuntimeLogsScope } from "./log-labels";
import {
  clearRuntimeLogsScopes,
  clearScopeMeta,
  isClearRuntimeLogsScope,
} from "./log-labels";

export function ClearLogsDialog({
  open,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  pending: boolean;
  error: unknown;
  onOpenChange: (open: boolean) => void;
  onConfirm: (scope: ClearRuntimeLogsScope) => void;
}) {
  const [scope, setScope] = useState<ClearRuntimeLogsScope>("before_7d");

  const errorMessage =
    error instanceof ApiError
      ? error.message
      : error
        ? "清理日志失败，请稍后重试。"
        : null;

  return (
    <AlertDialog.Backdrop
      isOpen={open}
      isDismissable={!pending}
      isKeyboardDismissDisabled={pending}
      onOpenChange={(next) => {
        if (!pending) onOpenChange(next);
      }}
    >
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-md">
          <AlertDialog.Header>
            <AlertDialog.Heading>清理运行日志</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="grid gap-4">
              <RadioGroup
                variant="secondary"
                value={scope}
                isDisabled={pending}
                onChange={(value) => {
                  if (isClearRuntimeLogsScope(value)) setScope(value);
                }}
              >
                <Label>选择清理范围</Label>
                <div className="grid gap-2.5 pt-1">
                  {clearRuntimeLogsScopes.map((item) => {
                    const meta = clearScopeMeta[item];
                    return (
                      <Radio key={item} value={item}>
                        <Radio.Content className="items-start">
                          <Radio.Control className="mt-0.5">
                            <Radio.Indicator />
                          </Radio.Control>
                          <div className="grid gap-0.5">
                            <Label>{meta.label}</Label>
                            <Description>{meta.description}</Description>
                          </div>
                        </Radio.Content>
                      </Radio>
                    );
                  })}
                </div>
              </RadioGroup>
              <Alert status="warning">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Description>
                    仅清理系统运行日志，不会删除安全审计记录和任务中心历史，也不会修改宿主机日志文件。
                  </Alert.Description>
                </Alert.Content>
              </Alert>
              {errorMessage && (
                <Alert role="alert" status="danger">
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Description className="wrap-anywhere">
                      {errorMessage}
                      {error instanceof ApiError && error.requestId
                        ? `（请求 ID ${error.requestId}）`
                        : ""}
                    </Alert.Description>
                  </Alert.Content>
                </Alert>
              )}
            </div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button
              variant="secondary"
              isDisabled={pending}
              onPress={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              variant="danger"
              isPending={pending}
              onPress={() => onConfirm(scope)}
            >
              确认清理
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  );
}
