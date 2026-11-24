import { useQuery } from "@tanstack/react-query";

import type { StrmFilters } from "./api";
import { getStrm, listStrm } from "./api";

export const strmKeys = {
  all: ["strm"] as const,
  list: (filters: StrmFilters) => ["strm", "list", filters] as const,
  detail: (id: string) => ["strm", "detail", id] as const,
};

export function useStrmFiles(filters: StrmFilters) {
  return useQuery({
    queryKey: strmKeys.list(filters),
    queryFn: ({ signal }) => listStrm(filters, signal),
    retry: false,
  });
}

export function useStrmDetail(id: string | undefined) {
  return useQuery({
    queryKey: strmKeys.detail(id ?? ""),
    queryFn: ({ signal }) => getStrm(id ?? "", signal),
    enabled: !!id,
    retry: false,
  });
}

export function strmLibraryPath(libraryId: string) {
  return `/strm?${new URLSearchParams({ library_id: libraryId })}`;
}
