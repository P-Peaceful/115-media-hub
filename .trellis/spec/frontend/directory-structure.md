# Frontend Directory Structure

## Current layout

[main.tsx](../../../frontend/src/main.tsx) composes StrictMode, providers and RouterProvider. [Project context](../project-context.md) documents the distinction between this foundation and future product screens.

```text
frontend/src/
  app/providers/         Query client and document theme
  app/layouts/           AppLayout shell
  app/router/            browser router and foundation/failure pages
  features/              named business areas, currently .gitkeep only
  shared/api/            request helper, tests and generated schema
  shared/stores/         persisted UI preferences
  shared/{hooks,lib,ui}/  currently .gitkeep only
  styles/                global import order and application tokens
  assets/                currently .gitkeep only
```

Concrete navigation examples are [router.tsx](../../../frontend/src/app/router/router.tsx) and [FoundationPages.tsx](../../../frontend/src/app/router/FoundationPages.tsx). `/` renders the foundation page, `*` renders not-found, and the layout supplies `errorElement`. No drive/task/settings feature route is implemented. A server deep-link shell response does not imply that React has a matching business page.

## Placement and naming

Keep composition in `app`, feature-specific screens/logic in the corresponding `features/{auth,browse,connections,libraries,logs,mediawall,metadata,overview,settings,strm,tasks}` folder, and genuinely shared infrastructure in `shared`. These are intended boundaries, not a preexisting feature module pattern. Avoid promoting a single screen's draft or widget into shared/global state prematurely.

Existing component files use PascalCase (`AppLayout.tsx`, `AppProviders.tsx`); infrastructure uses descriptive lowercase names (`client.ts`, `preferences.ts`) and tests use `.test.ts`. There are no barrel-export or custom feature-hook examples to copy. Keep code colocated until reuse is demonstrated.

The source of generated [schema.d.ts](../../../frontend/src/shared/api/generated/schema.d.ts) is [api/openapi.yaml](../../../api/openapi.yaml). Run `make generate`; do not edit generated declarations. The [Vite configuration](../../../frontend/vite.config.ts) proxies `/api`, `/health`, `/p` to `MEDIA_HUB_DEV_API_URL` or `http://127.0.0.1:8080`. Keep those server-reserved namespaces out of client page routing.

## Future screen boundaries and anti-patterns

[UI design](https://app.notion.com/p/401138389edc8143b487f666a58004f4) calls for stable-ID routes, one current directory with breadcrumbs/up/history, and URL-addressable detail views. Do not treat paths as object identity or build recursive expanded trees by default. These routes and controls still require a scoped implementation task.

Wrong: hand-edit a generated API type or add a successful business placeholder to the foundation page. Correct: update the shared contract and implement a real feature with explicit loading/failure/unknown states. See [type safety](type-safety.md) and [quality](quality-guidelines.md).
