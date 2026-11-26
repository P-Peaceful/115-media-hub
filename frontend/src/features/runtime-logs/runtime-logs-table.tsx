import { Button, Chip, Link, Table } from "@heroui/react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { RefObject } from "react";
import { useState } from "react";

import type { RuntimeLogEntry } from "./api";
import {
  formatRuntimeLogTime,
  formatTaskBadge,
  levelMeta,
  moduleLabels,
} from "./log-labels";

function hasExtraMetadata(entry: RuntimeLogEntry): boolean {
  return (
    Boolean(entry.error_code) ||
    Boolean(entry.request_id) ||
    Object.keys(entry.details).length > 0
  );
}

export function RuntimeLogsTable({
  entries,
  highlightErrors,
  scrollContainerRef,
}: {
  entries: readonly RuntimeLogEntry[];
  highlightErrors: boolean;
  scrollContainerRef: RefObject<HTMLDivElement | null>;
}) {
  const [expandedIds, setExpandedIds] = useState<ReadonlySet<string>>(
    () => new Set(),
  );

  function toggleExpanded(id: string) {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <Table>
      <Table.ScrollContainer
        ref={scrollContainerRef}
        className="max-h-[clamp(22rem,58vh,40rem)]"
      >
        <Table.Content
          aria-label="运行日志列表"
          className="min-w-[48rem]"
          selectionMode="none"
        >
          <Table.Header>
            <Table.Column id="occurred_at" className="w-44">
              时间
            </Table.Column>
            <Table.Column id="level" className="w-24">
              级别
            </Table.Column>
            <Table.Column id="module" className="w-28">
              模块
            </Table.Column>
            <Table.Column id="task" className="w-28">
              任务
            </Table.Column>
            <Table.Column id="message" isRowHeader>
              日志内容
            </Table.Column>
          </Table.Header>
          <Table.Body
            items={entries}
            dependencies={[highlightErrors, expandedIds]}
          >
            {(entry) => {
              const level = levelMeta[entry.level];
              const isHighlightedError =
                highlightErrors && entry.level === "error";
              const expandable = hasExtraMetadata(entry);
              const expanded = expandedIds.has(entry.id);
              const detailEntries = Object.entries(entry.details);

              const highlightCellClass = isHighlightedError
                ? " bg-danger-soft/60"
                : "";

              return (
                <Table.Row id={entry.id}>
                  <Table.Cell
                    className={`align-top font-mono text-xs whitespace-nowrap tabular-nums sm:text-sm${highlightCellClass}`}
                  >
                    {formatRuntimeLogTime(entry.occurred_at)}
                  </Table.Cell>
                  <Table.Cell
                    className={`align-top whitespace-nowrap${highlightCellClass}`}
                  >
                    <Chip size="sm" variant="soft" color={level.color}>
                      <Chip.Label>{level.label}</Chip.Label>
                    </Chip>
                  </Table.Cell>
                  <Table.Cell
                    className={`align-top whitespace-nowrap${highlightCellClass}`}
                  >
                    {moduleLabels[entry.module]}
                  </Table.Cell>
                  <Table.Cell
                    className={`align-top font-mono text-xs whitespace-nowrap sm:text-sm${highlightCellClass}`}
                  >
                    {entry.job_id ? (
                      <Link
                        href={`/tasks/${encodeURIComponent(entry.job_id)}`}
                        className="font-medium"
                      >
                        {formatTaskBadge(entry.job_id)}
                      </Link>
                    ) : (
                      <span className="text-muted">-</span>
                    )}
                  </Table.Cell>
                  <Table.Cell className={`align-top${highlightCellClass}`}>
                    <div
                      className="grid gap-1.5"
                      data-level={entry.level}
                      data-highlighted={isHighlightedError ? "true" : "false"}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <span className="wrap-anywhere text-foreground">
                          {entry.message}
                        </span>
                        {expandable && (
                          <Button
                            size="sm"
                            variant="ghost"
                            isIconOnly
                            aria-label={
                              expanded ? "收起日志详情" : "展开日志详情"
                            }
                            aria-expanded={expanded}
                            onPress={() => toggleExpanded(entry.id)}
                          >
                            {expanded ? (
                              <ChevronDown
                                aria-hidden="true"
                                className="size-4"
                              />
                            ) : (
                              <ChevronRight
                                aria-hidden="true"
                                className="size-4"
                              />
                            )}
                          </Button>
                        )}
                      </div>
                      {expanded && expandable && (
                        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 rounded-lg bg-surface-secondary px-3 py-2 text-xs">
                          {entry.operation && (
                            <>
                              <dt className="font-medium text-muted">操作</dt>
                              <dd className="font-mono wrap-anywhere">
                                {entry.operation}
                              </dd>
                            </>
                          )}
                          {entry.error_code && (
                            <>
                              <dt className="font-medium text-muted">错误码</dt>
                              <dd className="font-mono wrap-anywhere text-danger">
                                {entry.error_code}
                              </dd>
                            </>
                          )}
                          {entry.request_id && (
                            <>
                              <dt className="font-medium text-muted">
                                请求 ID
                              </dt>
                              <dd className="font-mono wrap-anywhere">
                                {entry.request_id}
                              </dd>
                            </>
                          )}
                          {detailEntries.map(([key, value]) => (
                            <div key={key} className="contents">
                              <dt className="font-medium text-muted">{key}</dt>
                              <dd className="font-mono wrap-anywhere">
                                {String(value)}
                              </dd>
                            </div>
                          ))}
                        </dl>
                      )}
                    </div>
                  </Table.Cell>
                </Table.Row>
              );
            }}
          </Table.Body>
        </Table.Content>
      </Table.ScrollContainer>
    </Table>
  );
}
