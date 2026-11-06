import { useQuery } from "@tanstack/react-query";

import { getReadiness } from "./api";

export function useReadiness() {
  return useQuery({
    queryKey: ["health", "ready"],
    queryFn: ({ signal }) => getReadiness(signal),
    retry: false,
    staleTime: 30_000,
  });
}
