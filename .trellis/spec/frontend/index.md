# Frontend Development Guidelines

English guidelines for the React foundation, reviewed on 2026-10-23. [Project context](../project-context.md) defines source authority and which Notion requirements remain future work.

| Guide | Use |
| --- | --- |
| [Feature workflows](feature-guidelines.md) | Product screens, file browsing, media libraries, STRM/task workflows, accessibility and safety |
| [Directory structure](directory-structure.md) | Application wiring, feature boundaries and generated files |
| [Components](component-guidelines.md) | HeroUI v3, styling and accessibility |
| [Hooks](hook-guidelines.md) | Current hooks and future query/event lifecycles |
| [State](state-management.md) | URL, Query, Zustand and local ownership |
| [Types](type-safety.md) | Request contract, generated schemas and validation limits |
| [Quality](quality-guidelines.md) | Build, lint, test and review baseline |

## Pre-Development Checklist

1. Read the active task PRD, [project context](../project-context.md) and [shared guides](../guides/index.md).
2. Read directory structure and quality; use feature workflows for product behavior, components for UI work, hooks/state for data flow, and types for API boundaries.
3. Inspect [package.json](../../../frontend/package.json), [router](../../../frontend/src/app/router/router.tsx) and [providers](../../../frontend/src/app/providers/AppProviders.tsx). Only foundation pages are implemented; feature folders are placeholders.
4. Define the owning state layer, loading/error/partial behavior and keyboard interactions before implementing a screen. Confirm the pinned HeroUI exports before adopting examples from another version.

## Quality Check

From repository root:

```sh
make check
pnpm --dir frontend build
make generate  # when OpenAPI changes
make smoke     # when embedded serving or integration behavior changes
```

The [Makefile](../../../Makefile) checks Go and frontend together; [frontend quality](quality-guidelines.md) explains individual commands and coverage limits. Check generated schema changes alongside API/Go changes, and use `make build` for the complete embedded application. No additional dependency or test is needed for a documentation-only correction.
