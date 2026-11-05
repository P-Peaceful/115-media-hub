# Bootstrap Project Development Guidelines

## Goal

Complete the existing bootstrap task by replacing the backend and frontend spec templates with actionable conventions for 115 Media Hub. The user authorized starting this task on 2026-11-06 and supplied the two Notion architecture checklists as the architectural source of truth.

## Evidence and current state

- `README.md` contains only the project name; there is no application source, `go.mod`, frontend package manifest, lockfile, or application CI configuration.
- `.trellis/spec/backend/` and `.trellis/spec/frontend/` contain unfilled templates.
- Backend architecture: https://app.notion.com/p/401138389edc811fabfcefbf05f26845 (edited 2026-11-06 04:40 Asia/Shanghai).
- Frontend architecture: https://app.notion.com/p/401138389edc814fb54be80c599600ef (edited 2026-11-06 04:42 Asia/Shanghai).
- The user requests conventional, consistent, readable frontend and backend code. Language/framework official guidance informs the new project conventions; there is no single universal style shared by all languages.

## Requirements

- Preserve the architecture selected in Notion, including the V1.0 safety constraints and on-demand frontend dependencies.
- Define clear naming, formatting, module boundaries, error handling, data ownership, tests, and review checks for future implementation.
- Every backend business module must have necessary structured operational logging, with module-specific event coverage, clear ownership, correlation, redaction, and review/test requirements.
- Cover all five backend and six frontend guideline topics and both indexes.
- Centralize shared API/SSE conventions and source provenance to keep frontend and backend guidance consistent.
- Mark suggested source paths and illustrative snippets as conventions for future implementation, never as existing code or verified runtime behavior.
- Write spec documents in English, matching the existing spec language requirement.
- Define future tooling gates without claiming that tools, dependencies, CI, or application tests have been configured or run.

## Scope

Edit the existing task artifacts and `.trellis/spec/` documentation. Do not initialize application packages, install dependencies, change Notion pages, modify Trellis runtime/skills, or include unrelated existing untracked files in commits.

## Acceptance criteria

- [x] Backend guidelines and index are complete and linked.
- [x] Backend logging defines necessary coverage for all 11 V1.0 business modules and future modules, including result/state/error records, volume policies, and test assertions.
- [x] Frontend guidelines and index are complete and linked.
- [x] Shared architecture/API guidance records source links, provenance, safety constraints, and explicit project decisions.
- [x] Examples cover Go error handling, conditional task claiming, React props, query cancellation/cache keys, and runtime type narrowing; every snippet is labeled illustrative.
- [x] Spec files contain no unfinished template text and local Markdown links resolve.
- [x] Frontend/backend naming, API errors, identifiers, timestamps, pagination, authentication, and SSE conventions agree.
- [x] Documentation whitespace and task-context validation pass; application checks are explicitly not applicable until scaffolding exists.

## Completion notes

The bootstrap produces an initial agreed baseline, not inferred historical code patterns. Future implementation tasks must add references to actual source and tests when those exist. Commit and archive follow the project's workflow after review of the concrete changes.
