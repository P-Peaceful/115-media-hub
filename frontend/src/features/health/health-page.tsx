import { Button, Card, Chip } from "@heroui/react";
import { CheckCircle2, CircleAlert, RefreshCw, Server } from "lucide-react";

import { ApiError } from "@/lib/api/errors";

import { useReadiness } from "./use-readiness";

export function HealthPage() {
  const health = useReadiness();
  let label = "正在检查";
  let description = "正在确认服务与数据库的连接状态…";
  if (health.isError) {
    label = "连接失败";
    description =
      health.error instanceof ApiError
        ? health.error.message
        : "检查失败，请稍后重试。";
  } else if (health.isSuccess) {
    label = "服务已就绪";
    description = "后端服务与数据库连接正常。";
  }
  return (
    <main className="page-shell">
      <header className="page-header">
        <p className="eyebrow">115 MEDIA HUB</p>
        <h1>欢迎使用媒体中心</h1>
        <p className="intro">
          集中管理你的云端媒体。从检查服务状态开始，让每一次连接清晰可见。
        </p>
      </header>
      <section aria-labelledby="health-title">
        <h2 id="health-title" className="mt-8 text-lg font-semibold">
          服务状态
        </h2>
        <Card className="health-panel">
          <Card.Header className="flex-row items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <Server aria-hidden="true" className="size-5" />
              <h3 className="font-semibold">后端连接</h3>
            </div>
            <Chip
              color={
                health.isError
                  ? "danger"
                  : health.isSuccess
                    ? "success"
                    : "default"
              }
              variant="soft"
            >
              {label}
            </Chip>
          </Card.Header>
          <Card.Content>
            <div
              role="status"
              aria-live="polite"
              aria-atomic="true"
              className="flex items-start gap-3 py-5"
            >
              {health.isSuccess && !health.isError && (
                <CheckCircle2
                  aria-hidden="true"
                  className="mt-1 size-5 shrink-0 text-success"
                />
              )}
              {health.isError && (
                <CircleAlert
                  aria-hidden="true"
                  className="mt-1 size-5 shrink-0 text-danger"
                />
              )}
              <div>
                <p className="status-description">{description}</p>
                {health.isFetching && !health.isPending && (
                  <p className="status-description">正在重新检查…</p>
                )}
                {health.isError &&
                  health.error instanceof ApiError &&
                  health.error.requestId && (
                    <p className="status-description break-all text-sm">
                      请求编号：{health.error.requestId}
                    </p>
                  )}
              </div>
            </div>
          </Card.Content>
          <Card.Footer>
            <Button
              variant="secondary"
              isPending={health.isFetching}
              onPress={() => {
                void health.refetch();
              }}
            >
              <RefreshCw aria-hidden="true" className="size-4" />
              {health.isFetching ? "检查中…" : "重新检查"}
            </Button>
          </Card.Footer>
        </Card>
      </section>
      <footer className="page-footer">115 Media Hub · 让媒体井然有序</footer>
    </main>
  );
}
