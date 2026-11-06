# Frontend Directory Structure

## Target layout

Use `frontend/` as the independent Vite application root. The [baseline](../guides/project-baseline.md) records source evidence and current repository status. Create feature directories only when implementing those features.

```text
frontend/src/app/                 app composition, providers, router, layouts
frontend/src/features/<feature>/  pages, components, hooks, API functions, local types
frontend/src/components/          shared product components
frontend/src/lib/api/             HTTP client, errors, runtime decoding
frontend/src/lib/api/generated/   types generated from api/openapi.yaml
frontend/src/lib/events/          SSE transport and event decoding
frontend/src/stores/              shared UI preferences only
frontend/src/styles/              Tailwind entry and shared design tokens
frontend/src/assets/              bundled static assets
```

Keep tests beside their subject (`*.test.ts`, `*.test.tsx`); place future browser scenarios in `frontend/e2e/`. Keep feature-specific code in its feature until a concrete second consumer needs a stable shared concept. Avoid a universal `utils.ts` or excessive barrel exports.

## Dependency direction

`app` composes features/shared modules; features use shared modules; shared modules never import features. Do not import another feature's private components or state; expose a deliberate public boundary when necessary. Keep HTTP calls and payload decoding in API modules and consume them through query hooks. Components must not redefine backend DTOs or open independent SSE connections per row.

## Naming

Use kebab-case paths (`library-list.tsx`, `use-library-list.ts`), PascalCase named component exports (`LibraryList`), camelCase functions/variables, and `use...` hook exports. Use `import type` for type-only dependencies. Prefer named exports; use default exports only where framework tooling requires them. Avoid `React.FC`, component class names for plain functions, and one-letter names beyond short conventional loop callbacks.

Configure a single `@/` alias for `frontend/src` in TypeScript, Vite, and tests together. Distinguish copied backend-embedded build output from `frontend/src` source; never edit generated artifacts to change behavior.
