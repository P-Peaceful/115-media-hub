# State Management

## Ownership

Read [project context](../project-context.md) and the adopted [frontend architecture](https://app.notion.com/p/401138389edc814fb54be80c599600ef). Ownership for future features is:

| State | Owner | Current implementation |
| --- | --- | --- |
| Server objects, runs, counts and request status | TanStack Query | Provider/defaults only; no queries |
| Connection/directory/library scope, filters, paging, detail deep link | URL / React Router | Root and wildcard routes only |
| Permitted UI preferences | Zustand persistence | Theme only |
| Form/wizard drafts, temporary page selection | Local/page state | No business forms or selection yet |

Do not store the same fact in multiple owners. Derive values instead of maintaining synchronized copies. [AppProviders](../../../frontend/src/app/providers/AppProviders.tsx) owns one client with query `staleTime: 30_000`, query `retry: 1` and mutation `retry: false`. These are application overrides, not a complete list of Query library defaults; no polling interval or SSE behavior is configured.

## Existing preference contract

[preferences.ts](../../../frontend/src/shared/stores/preferences.ts) defines:

```ts
type Theme = 'light' | 'dark'
interface Preferences {
  theme: Theme
  setTheme: (theme: Theme) => void
}
```

It starts with `theme: 'light'`, persists only `theme` under `115-media-hub-preferences` at version `1`, and exposes `setTheme`. The [layout](../../../frontend/src/app/layouts/AppLayout.tsx) toggles light/dark; the provider applies root class/attribute/color-scheme. There is no `system` variant or matchMedia subscription. System-following theme in [UI design](https://app.notion.com/p/401138389edc8143b487f666a58004f4) remains a recommendation requiring a separate task.

## Future scope and safety

The UI source requires single-directory navigation, stable-ID scope, current-page selection and deep-link detail history. Keep credentials out of URL/localStorage/Zustand. A detail drawer must derive from URL and work for direct entry as well as list navigation; no drawer exists today.

Represent successful empty, loading, failed, partial, stale, conflicting and unknown states separately. Unknown totals are not zero and cannot produce invented progress percentages. Keep records from partial results with affected scope and show cache age when relevant. Requests, connection state, object state and run state are different facts.

Destructive mutations must not optimistically declare completion. Future timeout/uncertain effects require reconciliation using an idempotency request identity rather than blind resubmission. Preview scope/version/evidence must be revalidated by the backend; hiding a button provides no authorization. Those behaviors are [functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8), not foundation features.

## Wrong vs correct and verification

Wrong: persist directory results, credentials or selected objects in the theme store. Correct: keep only allowed UI preferences there; use scoped Query data, URL scope and temporary stable IDs in page state when implemented. For future state work assert reload/back-forward/deep-link behavior, scope changes and unknown/partial outcomes. Current [request tests](../../../frontend/src/shared/api/client.test.ts) do not cover these state transitions or theme persistence.
