# Hook Guidelines

## Current examples

[AppProviders](../../../frontend/src/app/providers/AppProviders.tsx) creates the QueryClient once with lazy `useState`, reads a Zustand selector and updates document theme in `useEffect`. [preferences.ts](../../../frontend/src/shared/stores/preferences.ts) exports `usePreferences`; [AppLayout](../../../frontend/src/app/layouts/AppLayout.tsx) consumes it. These are the only current stateful hook patterns. `shared/hooks` is empty; no custom query hook, session hook or SSE hook exists. See [project context](../project-context.md).

```tsx
const [queryClient] = useState(() => new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1 }, mutations: { retry: false } },
}))
const theme = usePreferences((state) => state.theme)
```

Do not construct the client on every render: it would lose cache identity. Effects must include their dependencies; the current theme effect depends on `[theme]`. Follow React hook ordering and the configured [ESLint hook rules](../../../frontend/eslint.config.js).

## Future data hooks

[Frontend architecture](https://app.notion.com/p/401138389edc814fb54be80c599600ef) assigns server data to Query, not Zustand or ad hoc fetch effects. When adding a data hook, name it `use...`, keep feature-specific logic in that feature, and share only reusable behavior. Use the [request helper](../../../frontend/src/shared/api/client.ts) with a real generated contract; `request<T>` can return `undefined` and does not validate JSON at runtime.

The [UI design](https://app.notion.com/p/401138389edc8143b487f666a58004f4) requires keys scoped by connection/directory/library, filters and sort. Normal paged queries include page/cursor/page size; infinite queries use `pageParam` for successive cursors. Scope keys must prevent late old-directory results replacing a new scope. These are future design constraints; there is no real query-key example in this foundation.

SSE and polling are unimplemented. Suggested 2–3s polling is a recommendation, not a current default. Future subscriptions must clean up listeners/connections and reconcile events with Query; do not introduce a session/token guessing hook. Request cancellation may use the inherited `RequestInit.signal`, but there is no demonstrated query cancellation integration yet.

## Anti-patterns and assertions

Wrong: call `fetch` in an effect, copy the result into Zustand and call it the authoritative server state. Correct for future features: Query owns the request lifecycle/cache, URL owns scope and local state owns transient drafts. Do not promote destructive mutations into optimistic success; timeout/unknown results require server reconciliation.

For new hooks test scope changes, cancellation/cleanup and delayed responses, plus empty versus failed/partial states. Existing [client.test.ts](../../../frontend/src/shared/api/client.test.ts) tests only the request boundary, not hooks or SSE. Run `make check`; select a UI test strategy deliberately rather than claiming the current Node Vitest setup renders hooks.
