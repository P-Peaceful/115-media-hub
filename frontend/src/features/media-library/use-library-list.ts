import { useQuery } from "@tanstack/react-query";

import { listLibraries } from "./api";

export function useLibraryList() {
  return useQuery({
    queryKey: ["libraries", "list"],
    queryFn: ({ signal }) => listLibraries(signal),
    retry: false,
  });
}
