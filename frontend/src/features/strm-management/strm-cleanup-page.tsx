import { Button, Card, Table } from "@heroui/react";
import {
  CircleAlert,
  CircleCheck,
  CircleX,
  Database,
  File,
  FileText,
  Folder,
  Settings,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useNavigate } from "react-router";

import { PageBanner } from "@/components/page-banner";

import type { StrmCleanupPreview } from "./strm-demo";
import { formatStrmSize, previewStrmCleanup, strmLibraries } from "./strm-demo";
import { useStrmSession } from "./strm-session-context";
import { DemoBadge, MetricTile, StrmEmpty, StrmPanel } from "./strm-ui";

const countFormat = new Intl.NumberFormat("zh-CN");

const cleanupCategories = {
  keep: {
    label: "可保留（有效）",
    explanation: "源文件存在且播放链接可用",
    icon: CircleCheck,
    color: "text-success-soft-foreground",
  },
  remove: {
    label: "将清理（无效）",
    explanation: "已确认异常且符合清理条件",
    icon: CircleX,
    color: "text-danger-soft-foreground",
  },
  regenerate: {
    label: "可重新生成",
    explanation: "可通过重新生成更新文件内容",
    icon: CircleAlert,
    color: "text-warning-soft-foreground",
  },
};

function CleanupMetrics({ preview }: { preview: StrmCleanupPreview }) {
  return (
    <div className="grid grid-cols-2 gap-3 min-[1150px]:grid-cols-4">
      <MetricTile
        detail={`共 ${countFormat.format(preview.libraries.length)} 个媒体库`}
        icon={Folder}
        label="影响范围"
        tone="accent"
        value={
          <span className="block text-sm/5">
            {preview.libraries
              .map((library) => strmLibraries[library])
              .join("、") || "无"}
          </span>
        }
      />
      <MetricTile
        icon={File}
        label="文件数量"
        tone="accent"
        value={countFormat.format(preview.total)}
      />
      <MetricTile
        icon={Database}
        label="预计释放"
        tone="success"
        value={formatStrmSize(preview.releaseBytes)}
      />
      <MetricTile
        icon={Settings}
        label="操作类型"
        tone="accent"
        value={
          <span className="block text-sm/5">清理无效文件（保留有效）</span>
        }
      />
    </div>
  );
}

function CleanupDetails({ preview }: { preview: StrmCleanupPreview }) {
  return (
    <StrmPanel title="预览详情" icon={FileText}>
      <Table>
        <Table.ScrollContainer>
          <Table.Content
            aria-label="清理预览明细"
            className="min-w-[18.5rem] max-sm:[&_[role=columnheader]]:px-2 max-sm:[&_[role=gridcell]]:px-2 max-sm:[&_[role=rowheader]]:px-2 sm:min-w-[25rem]"
          >
            <Table.Header>
              <Table.Column isRowHeader>类型</Table.Column>
              <Table.Column>文件数量</Table.Column>
              <Table.Column>占用空间</Table.Column>
              <Table.Column>说明</Table.Column>
            </Table.Header>
            <Table.Body>
              {preview.rows.map(({ kind, count, bytes }) => {
                const {
                  label,
                  explanation,
                  icon: Icon,
                  color,
                } = cleanupCategories[kind];

                return (
                  <Table.Row id={kind} key={kind}>
                    <Table.Cell>
                      <span className="inline-flex items-center gap-1.5 sm:gap-2 sm:whitespace-nowrap">
                        <Icon
                          aria-hidden="true"
                          className={`size-4 shrink-0 ${color}`}
                        />
                        {label}
                      </span>
                    </Table.Cell>
                    <Table.Cell className="whitespace-nowrap tabular-nums">
                      {countFormat.format(count)}
                    </Table.Cell>
                    <Table.Cell className="whitespace-nowrap tabular-nums">
                      {formatStrmSize(bytes)}
                    </Table.Cell>
                    <Table.Cell className="text-muted">
                      {explanation}
                    </Table.Cell>
                  </Table.Row>
                );
              })}
            </Table.Body>
          </Table.Content>
        </Table.ScrollContainer>
      </Table>
    </StrmPanel>
  );
}

function CleanupRisk() {
  return (
    <Card className="overflow-hidden border border-danger/15 bg-danger-soft/45 p-0 shadow-sm">
      <Card.Header className="bg-danger-soft px-4 py-2.5">
        <Card.Title className="flex items-center gap-2 text-base font-semibold text-danger-soft-foreground">
          <TriangleAlert aria-hidden="true" className="size-5 shrink-0" />
          风险提示
        </Card.Title>
      </Card.Header>
      <Card.Content className="px-4 pt-0 pb-4 text-sm text-foreground">
        <ul className="list-disc space-y-1.5 ps-5 marker:text-danger-soft-foreground">
          <li>确认后会从本次演示中移除可清理的 STRM 文件。</li>
          <li>请核对媒体库范围和确认按钮中的文件数量。</li>
          <li>建议先进行有效性检查，再清理已确认异常的文件。</li>
        </ul>
      </Card.Content>
    </Card>
  );
}

export function StrmCleanupPage() {
  const navigate = useNavigate();
  const { files, cleanup, cancelCleanup, confirmCleanup } = useStrmSession();

  const heading = (
    <div className="strm-banner">
      <PageBanner
        title={
          <span className="relative block min-h-9 ps-12 sm:ps-16">
            <span className="absolute start-0 top-0 grid size-10 place-items-center rounded-full bg-warning-soft text-warning-soft-foreground sm:size-13">
              <TriangleAlert aria-hidden="true" className="size-6 sm:size-7" />
            </span>
            清理预览
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1 ps-12 sm:ps-16">
            请核对所选 STRM 文件的影响范围和清理风险。
            <DemoBadge />
          </span>
        }
      />
    </div>
  );

  if (!cleanup) {
    return (
      <div className="space-y-3">
        {heading}
        <StrmEmpty
          title="暂无清理预览"
          description="请从 STRM 管理中选择需要检查清理范围的文件。"
          href="/strm"
          label="返回 STRM 管理"
        />
      </div>
    );
  }

  const preview = previewStrmCleanup(files, cleanup.fileIds);
  const removableCount = preview.removableIds.length;
  const { returnTo } = cleanup;

  function cancel() {
    cancelCleanup();
    void navigate(returnTo, { replace: true });
  }

  function confirm() {
    if (confirmCleanup() > 0) {
      void navigate("/strm", { replace: true });
    }
  }

  return (
    <div className="space-y-3">
      {heading}
      <CleanupMetrics preview={preview} />
      <div className="grid items-start gap-3 min-[1150px]:grid-cols-[minmax(0,5fr)_minmax(0,3fr)]">
        <div className="min-w-0">
          <CleanupDetails preview={preview} />
        </div>
        <CleanupRisk />
      </div>
      <div className="flex flex-wrap justify-end gap-2 pt-1">
        <Button variant="outline" onPress={cancel}>
          取消
        </Button>
        <Button isDisabled={removableCount === 0} onPress={confirm}>
          <Trash2 aria-hidden="true" className="size-4" />
          确认清理（{countFormat.format(removableCount)} 个文件）
        </Button>
      </div>
    </div>
  );
}
