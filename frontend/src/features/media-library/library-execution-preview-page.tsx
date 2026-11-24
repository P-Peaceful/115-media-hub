import { Alert, Button, Card, Spinner } from "@heroui/react";
import { buttonVariants } from "@heroui/styles";
import { Info, Layers, Link2, Play, ScanSearch } from "lucide-react";
import { Link, useNavigate } from "react-router";

import {
  useSubmitTask,
  useTaskCapabilities,
} from "@/features/task-center/queries";
import { TaskError } from "@/features/task-center/task-ui";
import { ApiError } from "@/lib/api/errors";

import { useFullSyncPreview } from "./full-sync-preview-query";
import {
  existingFilePolicyLabels,
  linkFormatLabels,
  linkTypeLabels,
} from "./library-configuration-labels";
import {
  libraryDetailPath,
  useLibraryDetailContext,
} from "./library-detail-context";
import {
  canUsePreviewOutput,
  FullSyncPreviewResults,
} from "./library-full-sync-preview-results";
import {
  SyncPageHeader,
  SyncScope,
  SyncSection,
} from "./library-sync-components";
import {
  librarySyncHistoryPath,
  SyncTaskLink,
  useLibrarySyncHistory,
} from "./library-sync-history";
import { formatSTRMExample } from "./strm-format-example";
import { strmGenerationIneligibility } from "./strm-generation-eligibility";

export function LibraryExecutionPreviewPage() {
  const { item } = useLibraryDetailContext();
  const navigate = useNavigate();
  const ineligible = strmGenerationIneligibility(item);
  const capabilities = useTaskCapabilities(ineligible === null);
  const capability = capabilities.data?.find(({ type }) => type === "strm");
  const history = useLibrarySyncHistory(item.id, ineligible === null);
  const mutation = useSubmitTask();
  const activeTask = history.data?.items.find(
    ({ status }) =>
      status === "queued" || status === "running" || status === "paused",
  );
  const counts = history.data?.by_status;
  const hasActiveSync = !!(
    counts &&
    (counts.queued ?? 0) + (counts.running ?? 0) + (counts.paused ?? 0)
  );
  const canScan =
    ineligible === null &&
    capability?.available === true &&
    history.isSuccess &&
    !hasActiveSync;
  const preview = useFullSyncPreview(item.id, item.revision, canScan);
  const outdatedPreview =
    preview.data !== undefined && preview.data.revision !== item.revision;
  const currentPreview = outdatedPreview ? undefined : preview.data;
  const canConfirm =
    canScan &&
    preview.isSuccess &&
    preview.fetchStatus === "idle" &&
    currentPreview !== undefined &&
    canUsePreviewOutput(currentPreview.output);
  const needsAuthorization =
    preview.error instanceof ApiError &&
    (preview.error.code === "provider_not_configured" ||
      preview.error.code === "provider_auth_invalid");
  const detailPath = libraryDetailPath(item.id);

  return (
    <div className="space-y-4">
      <SyncPageHeader
        title="执行预览"
        description="预先扫描 115 媒体源并检查输出目录，核对结果后启动同步。"
        icon={Play}
        backToFullSync
      />
      <div className="grid gap-4 lg:grid-cols-2">
        <SyncSection title="同步范围" icon={Layers}>
          <SyncScope library={item} />
        </SyncSection>
        <SyncSection title="STRM 输出配置" icon={Link2}>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 rounded-xl bg-surface-secondary p-3 text-sm">
            <dt className="text-muted">链接类型</dt>
            <dd>{linkTypeLabels[item.link_type]}</dd>
            <dt className="text-muted">STRM 播放地址</dt>
            <dd className="wrap-anywhere">
              {item.strm_direct_domain || "未设置"}
            </dd>
            <dt className="text-muted">链接格式</dt>
            <dd>{linkFormatLabels[item.strm_link_format]}</dd>
            <dt className="text-muted">STRM 已存在时</dt>
            <dd>{existingFilePolicyLabels[item.strm_existing_file_policy]}</dd>
            <dt className="text-muted">文件编码</dt>
            <dd>{item.file_encoding}</dd>
          </dl>
          {item.link_type === "proxy" && item.strm_direct_domain && (
            <p className="mt-3 text-xs text-muted">
              <span className="mr-2">链接示例</span>
              <code className="wrap-anywhere">
                {formatSTRMExample(
                  item.strm_direct_domain,
                  item.strm_link_format,
                )}
              </code>
            </p>
          )}
        </SyncSection>
      </div>
      {canScan && (
        <SyncSection
          title="扫描结果"
          icon={ScanSearch}
          action={
            currentPreview && (
              <Button
                size="sm"
                variant="secondary"
                isDisabled={
                  preview.fetchStatus !== "idle" || mutation.isPending
                }
                onPress={() => void preview.refetch()}
              >
                重新扫描
              </Button>
            )
          }
        >
          <div className="space-y-3">
            {preview.isFetching && (
              <p
                role="status"
                className="flex items-center gap-2 text-sm text-muted"
              >
                <Spinner aria-hidden="true" size="sm" />
                正在扫描 115 媒体源并检查输出目录…
              </p>
            )}
            {preview.isPaused && (
              <p role="status" className="text-sm text-muted">
                等待网络连接以继续扫描…
              </p>
            )}
            <TaskError
              error={preview.error}
              {...(!currentPreview && { retry: () => void preview.refetch() })}
            />
            {needsAuthorization && (
              <Link
                className="inline-block text-sm font-medium text-accent hover:underline"
                to="/settings?tab=account"
              >
                管理 115 网盘授权
              </Link>
            )}
            {outdatedPreview && (
              <Alert role="alert" status="warning">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Description>
                    媒体库配置已更新，请重新加载后扫描。
                  </Alert.Description>
                  <div>
                    <Link
                      className={buttonVariants({
                        size: "sm",
                        variant: "secondary",
                      })}
                      reloadDocument
                      to={`${detailPath}/full-sync/execution-preview`}
                    >
                      重新加载配置
                    </Link>
                  </div>
                </Alert.Content>
              </Alert>
            )}
            {currentPreview && !preview.isError && (
              <FullSyncPreviewResults preview={currentPreview} />
            )}
          </div>
        </SyncSection>
      )}
      <Card>
        <Card.Content className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="flex min-w-0 flex-1 basis-full items-start gap-2 text-sm sm:basis-60">
              <Info
                aria-hidden="true"
                className="mt-0.5 size-4 shrink-0 text-accent"
              />
              确认后，任务会重新扫描当前媒体源，处理数量可能随源文件变化。
            </p>
            <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
              {mutation.isPending ? (
                <Button variant="secondary" isDisabled>
                  取消
                </Button>
              ) : (
                <Link
                  className={buttonVariants({ variant: "secondary" })}
                  to={`${detailPath}/full-sync`}
                >
                  取消
                </Link>
              )}
              <Button
                isDisabled={!canConfirm || mutation.isPending}
                isPending={mutation.isPending}
                onPress={() => {
                  if (!canConfirm || mutation.isPending) return;
                  mutation.mutate(
                    { type: "strm", libraryId: item.id },
                    {
                      onSuccess: (task) =>
                        void navigate(`/tasks/${encodeURIComponent(task.id)}`),
                    },
                  );
                }}
              >
                <Play aria-hidden="true" className="size-4" />
                {mutation.isPending ? "正在提交…" : "确认执行"}
              </Button>
            </div>
          </div>
          {ineligible ? (
            <p className="text-sm text-muted">
              {ineligible}{" "}
              <Link
                className="font-medium text-accent hover:underline"
                to={item.enabled ? `${detailPath}/edit` : detailPath}
                state={item.enabled ? { fromLibraryPage: true } : undefined}
              >
                {item.enabled ? "编辑媒体库配置" : "返回媒体库详情"}
              </Link>
            </p>
          ) : (
            <>
              {(capabilities.isPending || history.isPending) && (
                <p role="status" className="text-sm text-muted">
                  正在检查同步任务是否可用…
                </p>
              )}
              {capabilities.isSuccess && !capability?.available && (
                <p className="text-sm text-muted">
                  {capability?.reason ?? "全量同步暂不可用。"}
                </p>
              )}
              {hasActiveSync && (
                <p role="status" className="text-sm text-muted">
                  此媒体库已有全量同步任务正在处理。{" "}
                  {activeTask ? (
                    <SyncTaskLink id={activeTask.id} />
                  ) : (
                    <Link
                      className="font-medium text-accent hover:underline"
                      to={librarySyncHistoryPath(item.id)}
                    >
                      查看同步记录
                    </Link>
                  )}
                </p>
              )}
              <TaskError
                error={capabilities.error}
                retry={() => void capabilities.refetch()}
              />
              <TaskError
                error={history.error}
                retry={() => void history.refetch()}
              />
            </>
          )}
          <TaskError error={mutation.error} />
        </Card.Content>
      </Card>
    </div>
  );
}
