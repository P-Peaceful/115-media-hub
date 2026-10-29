# 115 Media Directory Browsing Implementation Plan

> **For agentic workers:** Implement the approved Trellis plan at `.trellis/tasks/10-28-115-media-directory-browsing/implement.md` in order. Keep API contract and generated files synchronized.

**Goal:** Implement on-demand single-page browsing of the current 115 directory to match the supplied single-directory design.

**Architecture:** Add an authenticated directory metadata and single-page entries API backed by the pinned 115 SDK request layer. Keep server data in TanStack Query and shareable directory, filter, search, and page state in the URL. Render the browse page within the existing authenticated shell.

**Tech Stack:** Go, Gin/OpenAPI generated server, React, TypeScript, TanStack Query, pinned `115driver` SDK.

**Spec:** `.trellis/tasks/10-28-115-media-directory-browsing/design.md`

## Constraints

- Load one requested page on demand; do not prefetch the first three pages or implement partial-page loading UI.
- Search and file-type filtering apply to direct children in the current directory.
- Do not implement backend business behavior for media-library creation or generation-scope inclusion.
- Do not add or run test suites unless explicitly requested. Perform applicable formatting, static analysis, type, lint, and build checks.
- Keep names, search terms, credentials, cookies, pickcodes, hashes, and raw responses out of logs.

## Ordered Work

1. Define OpenAPI request/response/error contracts and regenerate server/client types.
2. Implement a context-aware, timeout-bounded 115 adapter for stat and one-page listing; document non-obvious SDK mappings in Chinese.
3. Add browse service and authenticated HTTP handlers with input validation, safe errors, and structured operation logs; wire application dependencies.
4. Add frontend API/query state and URL synchronization for directory, search, type filter, and page.
5. Implement the screenshot-aligned table, breadcrumbs, selection, pagination, loading/error/empty states, and route integration. Keep out-of-scope actions disabled without fake success.
6. Review layout at the reference 1440×1024 size and narrow viewport; run static validation listed in the Trellis plan; inspect the final diff and sensitive-data boundaries.

## Review Focus

- SDK requests are constrained to the current directory and requested page; counts come from the API response, without full-directory scans.
- Page/query changes fetch only the requested page; failed refresh preserves the last successful page.
- Root and parent breadcrumb behavior is safe and consistent.
- API response and logs exclude sensitive 115 identifiers and credentials.
- Visual layout follows screenshot 05; screenshot 06's partial-loading treatment is not implemented.
