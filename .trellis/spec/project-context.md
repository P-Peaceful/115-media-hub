# Project Context and Source of Truth

## Sources

Reviewed for the 2026-10-23 Bootstrap Guidelines task and updated for administrator authentication on 2026-10-25:

| Source | Authority and use |
| --- | --- |
| [Functional requirements V1.0](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8) | Product scope, stable identity, ownership, safety rules and acceptance scenarios |
| [Backend architecture V1.0](https://app.notion.com/p/401138389edc811fabfcefbf05f26845) | Adopted backend stack, layering, deployment and reliability constraints |
| [Frontend architecture V1.0](https://app.notion.com/p/401138389edc814fb54be80c599600ef) | Adopted frontend stack, state ownership and optional dependency boundaries |
| [UI design V1.0](https://app.notion.com/p/401138389edc8143b487f666a58004f4) | Interaction requirements and recommended visual baseline; not evidence of implemented screens |

Local source code defines current behavior. Notion defines adopted requirements and recommendations, not delivered capability. If these differ, document the gap rather than inventing code or silently changing product scope. Read the live sources before future implementation when requirements may have changed.

## Implemented baseline

`backend/` is a Go module with Gin HTTP routing, GORM/SQLite, goose migrations, YAML/environment startup configuration, slog rotation and graceful shutdown. It requires one configured administrator and holds 24-hour sessions in memory. The first business table stores one active 115 connection with plaintext credential JSON in the local database; its QR authorization lifecycle is backend mediated and management APIs use the administrator session plus CSRF protection. `frontend/` is a React SPA with administrator login, a protected shared shell, connection management, router, theme preference, Query provider and request helper. The shared contract in [api/openapi.yaml](../../api/openapi.yaml) contains liveness, readiness, administrator authentication, and 115 connection operations. See the [authentication contract](backend/auth-contract.md) and [115 connection contract](backend/connection-guidelines.md).

See [development notes](../../docs/development.md), [architecture](../../docs/architecture.md), [backend entry point](../../backend/cmd/server/main.go) and [frontend entry point](../../frontend/src/main.tsx).

| Area | Current state | Requirement for subsequent implementation |
| --- | --- | --- |
| Module boundaries | Technical layers and feature directories exist, many only as placeholders | Gin context stays in HTTP; Service owns orchestration and transactions; GORM stays behind repositories; API DTOs and persistence models remain separate |
| Background work | Jobs directories exist without a scheduler or executor | Persist plans/runs/items/effects, claim conditionally, make retries bounded, cancel cooperatively, verify uncertain effects before restart recovery; use output/target locks and connection-level 115 throttling |
| Identity and filesystem | Filesystem adapter is a placeholder | Stable connection/file identity, explicit source/output/media-service/metadata path roles, containment and symlink checks, collision handling, content hashes and ownership |
| Destructive operations | No cleanup, overwrite, recovery or business APIs exist | Incomplete scans cannot imply deletion; cleanup defaults off and uses preview plus quarantine; preserve user modifications; verify upload results and restore state before enabling destructive automation |
| Playback and credentials | 115 authorization credentials are stored as plaintext JSON in the local database by explicit user decision; administrator sessions stay in memory; no playback resolution exists | Keep credentials out of API responses, browser storage, logs and task snapshots; protect database and backups as sensitive credential material; keep playback credentials separate |
| Media wall and metadata | Feature directories are placeholders | Both remain in V1.0; media-service APIs do not prove sidecars exist on disk; metadata mapping/baselines, conditional writes and conflict decisions require explicit evidence |
| UI and state | Protected router, login, shared shell, theme store, Query provider, request helper | Single-directory navigation, URL-preserved scope, Query-owned server data, bounded loaded lists, explicit partial/unknown/conflict states and accessible controls |
| Delivery | Frontend builds are embedded with `embedui`; Docker uses CGO and Debian | One application instance and local SQLite; `/storage/quarantine` stays outside media scan roots; configure actual shared paths before media integration |

These are implementation constraints for later tasks, not permission to implement all of V1.0 during guideline bootstrap. P1 features and suggested colors, viewport sizes, polling intervals or performance goals are not already-shipped or independently verified commitments.

## Shared review rules

- Preserve product scope: browsing, libraries, STRM, tasks, logs, media wall, metadata return and system management. Do not add transcoding, an independent scraper, video upload or bidirectional mirroring without a separate task.
- Represent empty, failed, partial, stale, cancelled, conflicting and uncertain outcomes separately. Never convert an unknown state to success.
- Treat a preview as a scoped/versioned result. Future destructive APIs must revalidate it on the server; hiding a button is not authorization.
- Use real file examples and distinguish executed checks from future acceptance scenarios. The functional document's large-index and client compatibility cases have not been run by the foundation test suite.
