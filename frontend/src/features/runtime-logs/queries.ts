import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { useAuth } from "@/features/auth/auth-provider";
import { listTasks } from "@/features/task-center/api";
import { ApiError } from "@/lib/api/errors";

import type { RuntimeLogEntry, RuntimeLogFilters } from "./api";
import {
  buildRuntimeLogStreamUrl,
  clearRuntimeLogs,
  decodeRuntimeLogClearedEvent,
  decodeRuntimeLogCreatedEvent,
  listRuntimeLogs,
} from "./api";
import type {
  ClearRuntimeLogsScope,
  RuntimeLogLevel,
  RuntimeLogModule,
  RuntimeLogRange,
} from "./log-labels";
import {
  isRuntimeLogLevel,
  isRuntimeLogModule,
  isRuntimeLogRange,
} from "./log-labels";

export const maxKeywordLength = 128;
export const defaultPageSize = 50;
export const maxLiveEntries = 200;

export const runtimeLogKeys = {
  all: ["runtime-logs"] as const,
  lists: ["runtime-logs", "list"] as const,
  list: (filters: RuntimeLogFilters) =>
    ["runtime-logs", "list", filters] as const,
  tasks: ["runtime-logs", "tasks"] as const,
};

export type StreamStatus = "idle" | "connecting" | "connected" | "reconnecting";

export function useRuntimeLogFilters() {
  const [params, setParams] = useSearchParams();
  const rawRange = params.get("range");
  const range: RuntimeLogRange = isRuntimeLogRange(rawRange) ? rawRange : "24h";
  const rawLevel = params.get("level");
  const level: RuntimeLogLevel | undefined = isRuntimeLogLevel(rawLevel)
    ? rawLevel
    : undefined;
  const rawModule = params.get("module");
  const module: RuntimeLogModule | undefined = isRuntimeLogModule(rawModule)
    ? rawModule
    : undefined;
  const jobId = (params.get("job_id") ?? "").trim().slice(0, 128);
  const keyword = (params.get("q") ?? "").slice(0, maxKeywordLength);
  const cursor = (params.get("cursor") ?? "").trim();

  const filters = useMemo<RuntimeLogFilters>(
    () => ({
      range,
      ...(level ? { level } : {}),
      ...(module ? { module } : {}),
      ...(jobId ? { job_id: jobId } : {}),
      ...(keyword.trim() ? { q: keyword.trim() } : {}),
      limit: defaultPageSize,
      ...(cursor ? { cursor } : {}),
    }),
    [range, level, module, jobId, keyword, cursor],
  );

  function update(values: Record<string, string>, replace = false) {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(values)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    void setParams(next, { replace, preventScrollReset: true });
  }

  return {
    filters,
    range,
    level: level ?? "all",
    module: module ?? "all",
    jobId,
    keyword,
    cursor,
    setRange: (value: RuntimeLogRange) =>
      update({ range: value === "24h" ? "" : value, cursor: "" }),
    setLevel: (value: string) =>
      update({ level: value === "all" ? "" : value, cursor: "" }),
    setModule: (value: string) =>
      update({ module: value === "all" ? "" : value, cursor: "" }),
    setJobId: (value: string) =>
      update({ job_id: value === "all" ? "" : value, cursor: "" }),
    setKeyword: (value: string) =>
      update({ q: value.slice(0, maxKeywordLength), cursor: "" }, true),
    setCursor: (value: string) => update({ cursor: value }),
    reset: () => void setParams({}, { preventScrollReset: true }),
  };
}

export function useRuntimeLogs(filters: RuntimeLogFilters) {
  return useQuery({
    queryKey: runtimeLogKeys.list(filters),
    queryFn: ({ signal }) => listRuntimeLogs(filters, signal),
    retry: false,
  });
}

export function useRuntimeLogTasks() {
  return useQuery({
    queryKey: runtimeLogKeys.tasks,
    queryFn: ({ signal }) =>
      listTasks(new URLSearchParams({ limit: "50" }), signal),
    retry: false,
    staleTime: 30_000,
  });
}

export function useClearRuntimeLogs() {
  const token = useAuth().data?.csrf_token;
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (scope: ClearRuntimeLogsScope) => {
      if (!token) {
        throw new ApiError("authentication_required", "请重新登录。");
      }
      return clearRuntimeLogs(token, scope);
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: runtimeLogKeys.all }),
  });
}

export function useRuntimeLogStream({
  enabled,
  filters,
  onEntry,
  onCleared,
}: {
  enabled: boolean;
  filters: RuntimeLogFilters;
  onEntry: (entry: RuntimeLogEntry) => void;
  onCleared: () => void;
}): StreamStatus {
  const { data: auth } = useAuth();
  const queryClient = useQueryClient();
  const [connection, setConnection] = useState<{
    url: string;
    state: "connected" | "reconnecting" | "closed";
  } | null>(null);
  const onEntryRef = useRef(onEntry);
  const onClearedRef = useRef(onCleared);
  const hadConnectedRef = useRef(false);

  useEffect(() => {
    onEntryRef.current = onEntry;
    onClearedRef.current = onCleared;
  }, [onEntry, onCleared]);

  const authenticated = Boolean(auth?.authenticated);
  const active = enabled && authenticated && typeof EventSource !== "undefined";
  const streamUrl = useMemo(() => buildRuntimeLogStreamUrl(filters), [filters]);

  useEffect(() => {
    if (!active) {
      hadConnectedRef.current = false;
      return;
    }
    const stream = new EventSource(streamUrl);

    function handleOpen() {
      const reconnected = hadConnectedRef.current;
      hadConnectedRef.current = true;
      setConnection({ url: streamUrl, state: "connected" });
      if (reconnected) {
        void queryClient.invalidateQueries({ queryKey: runtimeLogKeys.lists });
      }
    }

    function handleError() {
      setConnection({
        url: streamUrl,
        state:
          stream.readyState === EventSource.CLOSED ? "closed" : "reconnecting",
      });
    }

    function handleCreated(event: Event) {
      const entry = decodeRuntimeLogCreatedEvent(event);
      if (entry) onEntryRef.current(entry);
    }

    function handleCleared(event: Event) {
      if (!decodeRuntimeLogClearedEvent(event)) return;
      onClearedRef.current();
      void queryClient.invalidateQueries({ queryKey: runtimeLogKeys.lists });
    }

    stream.addEventListener("open", handleOpen);
    stream.addEventListener("error", handleError);
    stream.addEventListener("log.created", handleCreated);
    stream.addEventListener("log.cleared", handleCleared);

    return () => {
      stream.removeEventListener("open", handleOpen);
      stream.removeEventListener("error", handleError);
      stream.removeEventListener("log.created", handleCreated);
      stream.removeEventListener("log.cleared", handleCleared);
      stream.close();
      hadConnectedRef.current = false;
    };
  }, [active, streamUrl, queryClient]);

  if (!active) return "idle";
  if (connection?.url !== streamUrl) return "connecting";
  if (connection.state === "closed") return "idle";
  return connection.state;
}
