# Frontend Quality Guidelines

## Commands and enforced checks

The root [Makefile](../../../Makefile) and [package scripts](../../../frontend/package.json) are authoritative. Use the pinned pnpm version and frozen lockfile; Node minimum is `22.12.0`. [Project context](../project-context.md) defines the foundation's limited scope.

```sh
make check
pnpm --dir frontend typecheck
pnpm --dir frontend lint
pnpm --dir frontend peers check
pnpm --dir frontend test
pnpm --dir frontend build
```

`make check` already runs typecheck, lint, peer check and tests plus Go checks. Build additionally runs `tsc -b` and Vite. There is no configured coverage threshold or browser/E2E suite. [ESLint](../../../frontend/eslint.config.js) applies JS/TS recommended rules, React hook rules and refresh component-export rules, with zero warnings permitted; generated API files and dist are excluded. [TypeScript](../../../frontend/tsconfig.json) uses strict/unused checks. Do not suppress a boundary mismatch with `any` or broad assertions.

## Test evidence and limits

[client.test.ts](../../../frontend/src/shared/api/client.test.ts) uses Vitest with stubbed fetch and restores globals using `afterEach(() => vi.unstubAllGlobals())`. It covers JSON/credentials, 204, private error-body exclusion, explicit and in-memory CSRF headers, and external URL rejection. [auth-state.test.ts](../../../frontend/src/features/auth/auth-state.test.ts) covers expiry delay and safe return paths. A representative privacy assertion is:

```ts
await expect(request('/health/ready')).rejects.toMatchObject({
  status: 503, message: 'HTTP 503',
})
```

[Vite test config](../../../frontend/vite.config.ts) selects Node and `src/**/*.test.ts`; no DOM component testing setup or `.test.tsx` inclusion exists. Do not claim keyboard, rendering, accessibility, theme persistence, Query hooks or SSE were tested by this suite. Add meaningful behavioral tests when those features exist rather than tests that only repeat implementation details.

## Review checklist

- Preserve HeroUI v3 CSS order and semantic tokens; no v2 Provider or copied v2 component props. See [components](component-guidelines.md).
- Respect [state ownership](state-management.md); no credentials in persisted UI state, no duplicate server cache, no invented progress or destructive optimistic success.
- Preserve [request return/error behavior](type-safety.md); compiled types do not validate JSON.
- For API changes run `make generate` and review contract plus both generated outputs. Do not edit generated schema or commit `dist` build products.
- For embedded build/proxy/deep-link changes run `make smoke`. The backend reserves `/api`, `/p`, `/health` and rejects missing assets instead of returning the SPA shell; [router tests](../../../backend/internal/http/router/router_test.go) exercise those boundaries.

## Visual reference checks

When implementing or correcting a screen from a design file, open the exact reference view and the local page in a browser before changing layout. Compare the same screen regions and inspect their internal hierarchy, spacing, typography, colors and responsive behavior; text extraction or a scaled thumbnail alone can hide structural differences. After edits, inspect the local page again at a comparable viewport and report any remaining viewport-driven differences.

## Future feature acceptance

[UI design](https://app.notion.com/p/401138389edc8143b487f666a58004f4) calls for keyboard/screen-reader behavior, associated errors, overlay focus management, reduced motion, long Chinese paths, zoom/narrow layouts and bounded lists. [Functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8) include partial/unknown/conflict outcomes, safe previews, cancellation/restart and actual media-client playback. These require later feature tests; the foundation suite proves none of them. Suggested viewports, page sizes, polling intervals and system theme are recommendations, not measured current performance or default behavior.
