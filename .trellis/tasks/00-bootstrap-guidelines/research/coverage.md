# Source Coverage and Reconciliation

## Scope

This is guideline bootstrap, not V1.0 business implementation. All permanent spec text is English per the existing backend/frontend indexes. Shared source attribution and implemented-versus-required distinctions live in [project context](../../../spec/project-context.md).

| Notion source | Captured constraints | Permanent destination |
| --- | --- | --- |
| Backend architecture | Modular monolith, layer ownership, SQLite/goose/CGO, constructor assembly, logging, API/SPA delivery | Backend directory, database, errors, logging and quality guidelines |
| Frontend architecture | React/HeroUI/Tailwind, optional dependencies, Router/Query/Zustand separation, pagination before virtualization | Frontend directory, components, hooks, state, type safety and quality guidelines |
| Functional requirements | Stable identity, scan completeness, ownership/hash protection, quarantine, uncertain-effects recovery, metadata baselines, V1.0 wall/return scope | Project context; future constraints in backend/database and both quality guides |
| UI design | Current-page selection, direct-child directory search, URL scope, query identity, partial/unknown states, modal focus, semantic styling and reduced motion | Frontend component, hook, state, quality and shared context |

## Resolved differences

- Functional requirements leave frontend stack open; the dedicated frontend architecture and UI document make the adopted selection explicit. Use local manifests/locks for exact installed versions.
- UI recommends system-following theme; current `preferences.ts` only implements `light`/`dark` with light default. Record this gap without changing behavior.
- Backend selects a cron library; functional/UI documents reserve advanced user-authored Cron expressions for P1. Library choice is not permission to expose the P1 feature.
- Notion's full error/state/preview requirements are future contracts. Current health endpoints and request errors have a smaller concrete contract; do not invent a standardized business envelope.
- HeroUI has accessible primitives, but the foundation UI has not passed all UI acceptance scenarios. Component choice is not full accessibility certification.
- Files under service, repositories, adapters, jobs, auth and feature directories are largely placeholders. Source-file examples come from real infrastructure, not invented production modules.
- Blueprint preferences for colors, page sizes, breakpoints, polling and 10k/100k scale tests are not claims about measurements or completed features.

## Local evidence

Actual signatures and checks come from `backend/internal/config/config.go`, `backend/internal/database/database.go`, `backend/internal/http/handler/health.go`, `backend/internal/http/router/router.go`, `backend/internal/webui/handler.go`, `backend/internal/logging/logging.go`, `frontend/src/shared/api/client.ts`, `frontend/src/shared/api/client.test.ts`, `frontend/src/app/providers/AppProviders.tsx`, `frontend/src/shared/stores/preferences.ts`, `frontend/src/app/layouts/AppLayout.tsx`, `frontend/src/styles/globals.css`, `frontend/tsconfig.json`, `frontend/eslint.config.js`, `api/openapi.yaml`, `Makefile` and locked manifests.
