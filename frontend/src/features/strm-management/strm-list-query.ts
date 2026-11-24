import { useSearchParams } from "react-router";

import type { StrmFilters } from "./api";

export const pageSizes = { "25": "25 条", "50": "50 条", "100": "100 条" };
export const maxSearchLength = 256;
export const maxPage = 1_000_000;

export function useStrmListQuery() {
  const [params, setParams] = useSearchParams();
  const search = (params.get("q") ?? "").slice(0, maxSearchLength);
  const libraryId = (params.get("library_id") ?? "").slice(0, 128);
  const size = params.get("size") ?? "50";
  const limit = Object.hasOwn(pageSizes, size) ? Number(size) : 50;
  const requested = Number(params.get("page") ?? "1");
  const page =
    Number.isSafeInteger(requested) && requested >= 1 && requested <= maxPage
      ? requested
      : 1;
  const sort = params.get("sort") === "name" ? "name" : "generated";
  const order = params.get("order") === "asc" ? "asc" : "desc";
  const filters: StrmFilters = {
    ...(libraryId ? { library_id: libraryId } : {}),
    ...(search.trim() ? { q: search.trim() } : {}),
    sort,
    order,
    limit,
    page,
  };

  function change(values: Record<string, string>, replace = false) {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(values)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    void setParams(next, { replace, preventScrollReset: true });
  }

  return {
    filters,
    search,
    libraryId,
    sort,
    order,
    limit,
    page,
    setSearch: (value: string) => change({ q: value, page: "" }, true),
    setLibrary: (id: string) => change({ library_id: id, page: "" }),
    setSort: (column: "name" | "generated", direction: "asc" | "desc") =>
      change({ sort: column, order: direction, page: "" }),
    setSize: (value: string) => change({ size: value, page: "" }),
    setPage: (index: number) =>
      change({ page: index === 1 ? "" : String(index) }),
    reset: () => void setParams({}, { preventScrollReset: true }),
  };
}
