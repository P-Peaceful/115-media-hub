import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";

import { listTasks, type Task } from "@/features/task-center/api";
import { taskKeys } from "@/features/task-center/queries";
import {
  formatCount,
  formatDate,
  TaskError,
} from "@/features/task-center/task-ui";

function librarySyncSearch(libraryId: string) {
  return new URLSearchParams({ type: "strm", library_id: libraryId });
}

export function useLibrarySyncHistory(libraryId: string, enabled = true) {
  const search = librarySyncSearch(libraryId);
  return useQuery({
    queryKey: taskKeys.list(search.toString()),
    queryFn: ({ signal }) => listTasks(search, signal),
    retry: false,
    enabled,
  });
}

export function librarySyncHistoryPath(libraryId: string) {
  return `/tasks?${librarySyncSearch(libraryId)}`;
}

export function SyncTaskLink({
  id,
  label = "查看任务",
}: {
  id: string;
  label?: string;
}) {
  return (
    <Link
      className="whitespace-nowrap font-medium text-accent hover:underline"
      to={`/tasks/${encodeURIComponent(id)}`}
    >
      {label}
    </Link>
  );
}

export function SyncTaskTime({ task }: { task: Task }) {
  const time = task.started_at ?? task.created_at;
  return (
    <time dateTime={time} className="whitespace-nowrap tabular-nums">
      {formatDate(time)}
    </time>
  );
}

export function SyncTaskCounts({ task }: { task: Task }) {
  return (
    <span className="inline-flex flex-wrap gap-x-3 gap-y-1 tabular-nums">
      <span className="whitespace-nowrap">
        生成 {formatCount(task.succeeded_items)}
      </span>
      <span className="whitespace-nowrap">
        跳过 {formatCount(task.skipped_items)}
      </span>
      <span className="whitespace-nowrap">
        失败 {formatCount(task.failed_items)}
      </span>
    </span>
  );
}

export function SyncHistoryState({
  history,
}: {
  history: ReturnType<typeof useLibrarySyncHistory>;
}) {
  if (history.isPending)
    return (
      <p className="text-sm text-muted" role="status">
        正在加载同步记录…
      </p>
    );
  if (history.isError)
    return (
      <TaskError error={history.error} retry={() => void history.refetch()} />
    );
  if (history.data.items.length === 0)
    return <p className="text-sm text-muted">暂无全量同步记录。</p>;
  return null;
}
