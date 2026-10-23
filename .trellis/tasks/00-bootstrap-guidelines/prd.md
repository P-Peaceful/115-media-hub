# Bootstrap Guidelines

## Objective

Populate the first Trellis task with project-specific backend and frontend development guidelines for 115 Media Hub. The user explicitly requested completing this existing task using the Notion project documents on 2026-10-23.

## Sources

- [Project](https://app.notion.com/p/24d138389edc8086a364ccab282c0aaf)
- [Backend architecture](https://app.notion.com/p/401138389edc811fabfcefbf05f26845)
- [Frontend architecture](https://app.notion.com/p/401138389edc814fb54be80c599600ef)
- [Functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8)
- [UI design](https://app.notion.com/p/401138389edc8143b487f666a58004f4)

Source snapshots are under `research/`. Actual implementation and commands are verified against `backend/`, `frontend/`, `api/`, `deploy/`, `Makefile`, and `docs/`.

## Scope and constraints

- Documentation and this task's context/progress artifacts only; no application code, dependency, schema, deployment, or configuration changes.
- Keep the existing backend and frontend guideline topics. Update both indexes with Pre-Development Checklist and Quality Check sections.
- Follow the existing spec indexes' English documentation requirement.
- Describe implemented conventions as current behavior. Label Notion requirements that lack implementation as future constraints, and design recommendations as recommendations.
- Every topic must cite real source files and useful examples; do not invent a service/repository, custom query hook, validation library, error envelope, authentication flow, task executor, or virtualized table.
- Use `115-media-hub` and `MEDIA_HUB_` consistently. Preserve STRM as the file format and business concept.
- Do not modify Notion sources or unrelated working-tree files.

## Acceptance checklist

- [x] Fill backend guidelines: organization, database, error handling, logging, quality checks.
- [x] Fill frontend guidelines: organization, components, hooks, state management, type safety, quality checks.
- [x] Add verified examples and source traceability; indexes are actionable and no template placeholders remain.

## Verification

- Check local Markdown links and referenced source paths, example accuracy, placeholder removal, implementation/planning distinction, and source coverage.
- Run `task.py validate` for curated manifests and `make check` for the project baseline.
- Independent Trellis review checks the complete documentation scope.
- Request the workflow's one-shot commit confirmation only after the complete result is reviewable; do not stage unrelated untracked files. Archive/journal wrap-up follows the agreed commit workflow.
