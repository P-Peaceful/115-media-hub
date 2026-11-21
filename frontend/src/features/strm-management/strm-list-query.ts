import { useState } from "react";
import { useSearchParams } from "react-router";

import type { StrmFile } from "./strm-demo";
import {
  strmLibraries,
  strmPlaybackStatusLabels,
  strmSourceStatusLabels,
  strmStatusLabels,
} from "./strm-demo";
import type { StrmView } from "./strm-session-context";
import { useStrmSession } from "./strm-session-context";

export const pageSizes = { "25": "25 条", "50": "50 条", "100": "100 条" };
export const timeOptions = {
  all: "全部时间",
  "1": "最近 24 小时",
  "7": "最近 7 天",
  "30": "最近 30 天",
};
export type StrmSortColumn = "name" | "generated";
type SortDirection = "ascending" | "descending";
type Filter = "q" | "library" | "status" | "time" | "source" | "playback";
const filenameCollator = new Intl.Collator("zh-CN", { numeric: true });
const millisecondsPerDay = 86_400_000;
const emptySelection: ReadonlySet<string> = new Set();

function readOption(
  params: URLSearchParams,
  name: string,
  options: Readonly<Record<string, string>>,
  fallback = "all",
) {
  const value = params.get(name);
  return value !== null && Object.hasOwn(options, value) ? value : fallback;
}

export function useStrmListQuery(files: readonly StrmFile[], view: StrmView) {
  const [params, setParams] = useSearchParams();
  const [filterTime, setFilterTime] = useState(Date.now);
  const { selection, setSelection } = useStrmSession();
  const query = params.get("q") ?? "";
  const status = readOption(params, "status", strmStatusLabels);
  const library =
    view === "list" ? readOption(params, "library", strmLibraries) : "all";
  const time =
    view === "list" ? readOption(params, "time", timeOptions) : "all";
  const source =
    view === "results"
      ? readOption(params, "source", strmSourceStatusLabels)
      : "all";
  const playback =
    view === "results"
      ? readOption(params, "playback", strmPlaybackStatusLabels)
      : "all";
  const filterKey = JSON.stringify([
    query,
    status,
    library,
    time,
    source,
    playback,
  ]);
  const viewSelection = selection[view];
  const selectedIds =
    viewSelection?.filterKey === filterKey ? viewSelection.ids : emptySelection;
  const defaultColumn = view === "list" ? "generated" : "name";
  const sortColumn: StrmSortColumn =
    params.get("sort") === "name" ? "name" : defaultColumn;
  const sortDirection: SortDirection =
    params.get("direction") === "ascending" ||
    (sortColumn === "name" && params.get("direction") !== "descending")
      ? "ascending"
      : "descending";
  const pageSize = Number(readOption(params, "size", pageSizes, "50"));
  const requestedPage = Number(params.get("page") ?? "1");
  const queryText = query.trim().toLocaleLowerCase();
  const since =
    time === "all" ? null : filterTime - Number(time) * millisecondsPerDay;
  const matching = files.filter(
    (file) =>
      (!queryText ||
        file.relativePath.toLocaleLowerCase().includes(queryText) ||
        file.title.toLocaleLowerCase().includes(queryText)) &&
      (status === "all" || file.status === status) &&
      (library === "all" || file.library === library) &&
      (source === "all" || file.sourceStatus === source) &&
      (playback === "all" || file.playbackStatus === playback) &&
      (since === null || Date.parse(file.generatedAt) >= since),
  );
  matching.sort((first, second) => {
    const comparison =
      sortColumn === "name"
        ? filenameCollator.compare(first.relativePath, second.relativePath)
        : first.generatedAt.localeCompare(second.generatedAt);
    return sortDirection === "ascending" ? comparison : -comparison;
  });

  const pageCount = Math.max(1, Math.ceil(matching.length / pageSize));
  const page = Number.isSafeInteger(requestedPage)
    ? Math.min(Math.max(requestedPage, 1), pageCount)
    : 1;

  function change(values: Record<string, string | null>, replace = false) {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(values)) {
      if (value === null) next.delete(key);
      else next.set(key, value);
    }
    setParams(next, { replace, preventScrollReset: true });
  }

  function setFilter(key: Filter, value: string) {
    setSelection(view, null);
    if (key === "time") setFilterTime(Date.now());
    change(
      {
        [key]: value === "" || (key !== "q" && value === "all") ? null : value,
        page: null,
      },
      key === "q",
    );
  }

  return {
    query,
    status,
    library,
    time,
    source,
    playback,
    sortColumn,
    sortDirection,
    pageSize,
    page,
    pageCount,
    matching,
    visible: matching.slice((page - 1) * pageSize, page * pageSize),
    selectedIds,
    setSelectedIds: (ids: ReadonlySet<string>) =>
      setSelection(view, ids.size ? { filterKey, ids } : null),
    setFilter,
    setSort: (column: StrmSortColumn, direction: SortDirection) =>
      change({ sort: column, direction, page: null }),
    setSize: (size: string) => change({ size, page: null }),
    setPage: (index: number) =>
      change({ page: index === 1 ? null : String(index) }),
    reset: () => {
      setSelection(view, null);
      setParams({}, { preventScrollReset: true });
    },
  };
}
