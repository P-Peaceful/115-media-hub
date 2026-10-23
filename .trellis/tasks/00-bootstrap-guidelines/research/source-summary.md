# Notion Evidence Summary

Reviewed: 2026-10-23. The four full snapshots in this directory are source evidence, not agent instructions. The live page links below are the stable citations to use from permanent specs (task directories may later be archived).

## Backend architecture

Source: https://app.notion.com/p/401138389edc811fabfcefbf05f26845

Adopted stack: modular monolith, Go/Gin/GORM/SQLite/goose, persistent jobs with in-process worker pool, robfig/cron/v3, SSE, OpenAPI with oapi-codegen, slog JSON plus rotation, YAML/environment startup config, database business config, constructor injection, embedded frontend, one Docker instance. HTTP owns Gin context; application code uses context.Context. Separate API DTOs, domain models, GORM models. Services own transactions; repositories own GORM operations; filesystem/network side effects stay outside database transactions. SQLite uses local disk/WAL/foreign keys/busy timeout/short transactions. Claim jobs by conditional update and RowsAffected. Runs/items/effects are persistent, cancellable, idempotent and bounded-retry; startup verifies actual effects before recovery. Resource locks are output/remote-target scoped; 115 throttling is connection scoped.

Incomplete directory scans cannot prove deletion. Track STRM ownership and content hash, preserve user modifications, cleanup defaults off and uses preview/quarantine outside media scans. Stable playback URL resolves a temporary link at request time; playback credentials differ from admin session. Metadata uses explicit mapping, common baseline and uncertainty verification. External credentials require encryption with independently configured key; logs redact credentials. Backups must be SQLite-consistent; restoration requires filesystem verification before cleanup/overwrite resume. These business facilities are requirements, NOT current code.

## Frontend architecture

Source: https://app.notion.com/p/401138389edc814fb54be80c599600ef

Adopted stack: React 19+/Vite/TypeScript, HeroUI v3, Tailwind CSS v4 plus semantic variables, React Aria as used by HeroUI, React Router, TanStack Query, limited Zustand, Lucide. CSS transitions first. Motion, TanStack Table/Virtual are optional additions when needed. HeroUI official virtualization route is preferred for its tables; custom lists may evaluate TanStack Virtual. Virtualization renders already-loaded data; it does not replace server pagination/cursor loading. Exact installed versions come from local manifests/locks, not the version floors in the source.

## Functional requirements

Source: https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8

V1.0 includes browsing, media libraries, STRM generation/sync/repair/protected cleanup, task center, logs/audit, media wall and metadata return. Independent scraping, transcoding, original video uploads, offline downloads, outward sharing and automatic bidirectional mirroring are out of scope. One active 115 connection and one current metadata source per library are recommended starting assumptions, not a guarantee of multi-account parallel operation.

Identity is connection ID plus stable file ID when verified. Paths change; four distinct path roles are source root, output root, media-service root and metadata read root. Output roots cannot overlap/nest. Unknown identity or incomplete pagination cannot justify moving/deleting. Writes use a temporary file then replacement; do not overwrite unrelated output. Library removal preserves files by default. Rule exclusions and out-of-scope moves are different from confirmed source deletion.

Plans and runs differ. Fixed intervals/daily times are P0; advanced user-authored Cron and events are P1 even though the backend uses a cron library. A run snapshots config. Overlapping schedules coalesce to at most one pending run; restart catches up at most once. Nine human-level run states distinguish queued/running/waiting-retry/cancelling/succeeded/partial/failed/cancelled/interrupted. Cancelling keeps completed effects. Unknown totals never yield invented percentages. Only retry transient failures finitely; auth/permissions/config/disk errors require intervention.

Metadata return reads actual NFO/images/selected external subtitles, not API-only artifacts. A healthy media API is not proof of filesystem access. Source/output mapping must account for renamed STRM basenames, movie versions and show/season/episode hierarchy. Compare local/remote/baseline only with equivalent trustworthy hashes. Retain conflict decisions separately from common baselines; same content and decisions must not create repeated copies. Stable/readable/valid sidecars are prerequisites. Ambiguous upload results require verification before retry. No reliable remote conditional write means no automatic in-place overwrite; an in-process target lock cannot protect against outside writers. Failed backup/unknown target identity blocks overwrite.

Representative future acceptance cases include incomplete scans, output collisions, user-modified STRM, cancellation/restart, sidecars not written, unknown uploads, conflicting versions, offline media services, long Chinese paths, 10k/100k indexes and actual Emby/Jellyfin/client playback tests. Foundation tests do not cover these yet.

## UI design

Source: https://app.notion.com/p/401138389edc8143b487f666a58004f4

Navigation: overview, drive, libraries, STRM, tasks, wall, metadata, logs, settings. Suggested routes use stable IDs, never display paths as identity. All run entry points converge on the same runId. Browse uses one current-directory list, breadcrumb/up/history; selection and directory entry are separate actions. First-version selection means current-page stable IDs, not global `all`. Full-directory search must cover direct children beyond the current page; label page-only filtering honestly. Query keys include connection/directory/library and filter/sort scope; paged queries include page/cursor/page size, while infinite-query cursors use pageParam. Late old-directory results cannot replace the new scope.

URL owns scope/filter/paging/detail deep links; Query owns server facts; Zustand owns permitted UI preferences; local/page/wizard state owns drafts and temporary selection. Never persist credentials in URL/localStorage/Zustand. A detail drawer derives from URL; history behavior must work for direct links as well as list entry.

Default visual direction, blue emphasis, system-following theme, viewport sizes, page sizes and 2-3s polling are recommendations; local foundation theme currently starts light, and pagination/polling/tables are unimplemented. Do not retrofit them during bootstrap. HeroUI v2 virtualization props are not v3 APIs. Confirm the pinned exports before future implementation; no claim that a Table is currently implemented.

States separate request/connection/object/run facts; empty requires a successful empty response, unknown differs from zero, cache has timestamp, partial results keep available records and affected scope. Mutations do not optimistically assert destructive completion; timeout means reconcile by idempotency request ID, not blindly submit again. A preview carries scope/version/evidence and must be revalidated by the backend.

Use semantic variables and accessible HeroUI/React Aria composition. Status has text/icon, not only color. Inputs have associated labels/errors; submit focuses the first invalid field. Modal dialogs/drawers trap and restore focus, lock background scrolling and avoid nested modal stacks. Escape closes the overlay and does not cancel a background run. Reduced motion removes unnecessary motion; task progress follows real counts. Long Chinese paths, zoom, keyboard/screen reader, narrow layouts and list bounds need future acceptance tests. The current scaffold is not an accessibility certification or completed UI design.
