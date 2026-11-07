# Frontend Development Guidelines

## Scope and authority

Read the [project baseline](../guides/project-baseline.md) first for Notion sources, stack decisions, shared API conventions, and implementation status. The React SPA is rooted in `frontend/`; login and first-run setup guard the application; the health view is the current authenticated home. Feature examples without source links describe future business modules.

## Guidelines

| Guide | Responsibility |
| --- | --- |
| [Authentication](./authentication.md) | Login/setup routes, session state, recovery guidance and visual acceptance |
| [Directory structure](./directory-structure.md) | Feature ownership, naming, dependency boundaries |
| [Components](./component-guidelines.md) | Props, composition, HeroUI v3, styling, accessibility |
| [Hooks](./hook-guidelines.md) | Query keys, cancellation, effects, mutations |
| [State management](./state-management.md) | Query, URL, component, Zustand, SSE ownership |
| [Type safety](./type-safety.md) | Strict TypeScript, API types, runtime validation |
| [Quality](./quality-guidelines.md) | Formatting, lint, testing, performance, review |

## Pre-Development Checklist

1. Read the current task and project baseline.
2. Read directory structure, components, type safety, and quality before frontend changes.
3. Read hooks and state management for fetching, routing, subscriptions, or shared state changes.
4. Check the exact installed HeroUI v3 documentation before using component APIs; do not use v2 examples.
5. Read authentication for login/setup/session/protected-state changes. For API/SSE changes, read the [cross-layer guide](../guides/cross-layer-thinking-guide.md), update OpenAPI, and regenerate types before consumers.

## Quality Check

Follow the [quality gate](./quality-guidelines.md). Review query/URL/UI state ownership, contract decoding, keyboard access, themes, and async states. Report which checks actually ran; use the commands in `frontend/package.json` and root `Makefile`.
