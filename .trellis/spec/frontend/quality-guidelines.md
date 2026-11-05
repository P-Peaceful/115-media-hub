# Frontend Quality Guidelines

## Style and readability

Use TypeScript and React official guidance, ESLint for correctness, and Prettier for formatting. Initialization must establish ESLint flat config with TypeScript type-aware recommended rules, React Hooks rules, and accessible JSX checks compatible with the installed stack. Avoid overlapping formatter lint rules.

Project formatter decisions: two-space indentation, double quotes, semicolons, trailing commas where supported, and an 80-column preferred print width controlled by Prettier. Let tools format JSX rather than hand-aligning it. Keep imports grouped as external, internal alias, then relative; separate type-only imports and use one deterministic ordering rule once configured. Use LF, UTF-8, and final newlines. Go retains `gofmt` tabs rather than adopting JavaScript indentation.

Prefer named function components, early returns for meaningful states, small cohesive modules, and explicit domain names. Avoid nested ternaries for complex rendering, speculative abstractions, disabled Hooks rules, `console.log` debugging, and whole-object debug dumps containing private metadata.

## Future tooling gate

No `web/package.json`, lockfile, test harness, or frontend CI exists yet. Initialization should select a single package manager (default project choice: pnpm), pin it in `packageManager`, commit its lockfile, and define these scripts from `web/`:

| Script | Required behavior |
| --- | --- |
| `pnpm format:check` | Prettier verifies supported source/config files without writing |
| `pnpm lint` | ESLint completes with zero warnings |
| `pnpm typecheck` | TypeScript project checks all application/test source without emitting |
| `pnpm test:run` | Vitest and React Testing Library run once, not watch mode |
| `pnpm build` | Production Vite build suitable for backend embedding |
| `pnpm test:e2e` | Playwright covers implemented critical browser flows |

CI uses a frozen lockfile install and executes configured gates. Browser tests become required as critical flows exist. Contract generation must be deterministic and checked for drift once generators are installed. Do not claim these scripts ran or CI was configured as part of this documentation task.

## Tests and interaction checks

- Test observable user behavior using roles/labels and realistic input with React Testing Library. Avoid broad snapshots and tests that mirror component internals.
- Use Vitest for pure transformations, guards, hooks, and state. Use one consistent API mocking boundary (prefer MSW during initialization) and isolate QueryClient/store state between tests.
- Test loading/empty/error/success, mutation pending/failure, invalid responses, and query cancellation where relevant. Test session expiry and SSE reconnect/cleanup with deterministic controls.
- Browser scenarios cover implemented login/logout, directory traversal/history, library/job lifecycle, and destructive action previews. Include keyboard access, focus restoration, light/dark, narrow layouts, long names, and reduced motion as applicable.
- Frontend error messages must agree with the backend envelope. Never show an error as an empty successful result or let stale async responses rehydrate protected data after logout.

## Performance and dependency checks

Paginate large directories; measure before adding virtualization. Use HeroUI's documented v3 table approach for the pinned version and add TanStack Table/Virtual only for demonstrated gaps. Do not introduce Motion just to animate basic hover/focus. Query cache retention, SSE coalescing, and stable row IDs matter before blanket memoization.

## Review gate

Read the [baseline](../guides/project-baseline.md), [components](./component-guidelines.md), [hooks](./hook-guidelines.md), [state](./state-management.md), and [types](./type-safety.md) relevant to the change. Verify the actual package versions, generated contract, state owner, accessible behavior, async states, and responsive themes. Record commands/results and applicable limitations. Documentation-only changes require consistency, links, and whitespace review rather than application tests.
