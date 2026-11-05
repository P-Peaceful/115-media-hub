# Hook Guidelines

## Ownership and effects

Call hooks unconditionally at component/custom-hook top level. Name hooks `use...`; extract them to share a cohesive stateful behavior, not to hide a few unrelated expressions. Rendering and query functions must be free of unrelated writes.

Use TanStack Query for server state. Effects synchronize external systems such as SSE or document settings; derive ordinary values during rendering and perform user actions in event handlers/mutations. Keep exhaustive dependency checks enabled. Clean up listeners, subscriptions, timers, and abortable work; development StrictMode must not create duplicate long-lived subscriptions.

## Query conventions

Feature API modules own HTTP and decoding; hooks own cache keys/options. Include all data-changing parameters (directory, filter, cursor, sort, limit) in serializable stable query keys; exclude secrets. Define key factories near the feature and reuse them for invalidation.

Illustrative TanStack Query v5 pattern; `listLibraries` is a future API function that accepts and passes `signal` to fetch and returns a decoded response:

```tsx
import { useQuery } from "@tanstack/react-query";
import { listLibraries } from "./api";

export const libraryKeys = {
  all: ["libraries"] as const,
  list: (cursor: string | null) => ["libraries", "list", { cursor }] as const,
};

export function useLibraryList(cursor: string | null) {
  return useQuery({
    queryKey: libraryKeys.list(cursor),
    queryFn: ({ signal }) => listLibraries({ cursor, signal }),
  });
}
```

Set stale times and retry policies according to data volatility. Do not retry authorization/validation failures; cap retries for transient read failures. Automatic mutation retry is disabled by default; enable it only with a documented backend idempotency contract. Directory lists use API pagination/cursors; virtualization does not fetch missing data.

## Mutations and event updates

Return mutation promises and invalidate/update the relevant keys after success. Error and pending states must reach UI. Avoid optimistic updates for irreversible or conflict-sensitive file operations; if used elsewhere, define rollback and reconcile with server results.

The app owns the SSE connection; feature hooks subscribe through the event module. Decode once, coalesce high-frequency invalidations, and refetch on reconnect according to the [baseline](../guides/project-baseline.md#authentication-and-event-boundaries). Do not maintain a second authoritative job store from SSE messages.

## Verification

Use isolated QueryClients in tests; control retry/timers to keep tests deterministic. Verify parameter changes select the correct cache entry, cancellation reaches the HTTP call, mutation success invalidates the intended data, and effect cleanup works across remount/logout.

Reference: [TanStack query keys](https://tanstack.com/query/latest/docs/framework/react/guides/query-keys).
