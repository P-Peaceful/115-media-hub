import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router";

import { useAuth } from "@/features/auth/auth-provider";
import { listLibraries } from "@/features/media-library/api";
import { ApiError } from "@/lib/api/errors";

import { actOnTask, listCapabilities } from "./api";
import type { TaskAction } from "./task-kinds";

export const taskKeys = {
  all: ["tasks"] as const,
  lists: ["tasks", "list"] as const,
  list: (search: string) => ["tasks", "list", search] as const,
  detail: (id: string) => ["tasks", "detail", id] as const,
};
export const scheduleKeys = {
  all: ["schedules"] as const,
  list: (search: string) => ["schedules", "list", search] as const,
  detail: (id: string) => ["schedules", "detail", id] as const,
};
const capabilityKey = ["task-capabilities"] as const;

export function useTaskCapabilities() {
  return useQuery({
    queryKey: capabilityKey,
    queryFn: ({ signal }) => listCapabilities(signal),
    retry: false,
  });
}
export function useTaskLibraries() {
  return useQuery({
    queryKey: ["libraries", "list"],
    queryFn: ({ signal }) => listLibraries(signal),
    retry: false,
  });
}

export function useTaskSearch() {
  const [search, setSearch] = useSearchParams();
  const cursor = search.get("cursor") ?? "";

  function change(key: string, value: string, dropCursor = true) {
    const next = new URLSearchParams(search);
    if (dropCursor) next.delete("cursor");
    if (value) next.set(key, value);
    else next.delete(key);
    setSearch(next);
  }

  return { search, cursor, change };
}

export function useTaskCsrf() {
  const token = useAuth().data?.csrf_token;
  return () => {
    if (!token) throw new ApiError("authentication_required", "请重新登录。");
    return token;
  };
}

export function useTaskAction() {
  const csrf = useTaskCsrf();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: TaskAction }) =>
      actOnTask(csrf(), id, action),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: taskKeys.all }),
  });
}
