# Commit plan — approved

User requested completion and archival on 2026-10-30. The task workflow requires a confirmed work commit before archive/journal commits.

## Proposed work commit

`feat: add paginated 115 directory browsing`

Confirmed directory browsing implementation paths:

- `.trellis/spec/frontend/feature-guidelines.md`
- `api/openapi.yaml`
- `backend/internal/adapter/drive115/adapter.go`
- `backend/internal/adapter/drive115/errors.go`
- `backend/internal/app/app.go`
- `backend/internal/http/generated/api.gen.go`
- `backend/internal/http/router/router.go`
- `frontend/src/app/router/router.tsx`
- `frontend/src/shared/api/client.ts`
- `frontend/src/shared/api/generated/schema.d.ts`
- `backend/internal/adapter/drive115/browse.go`
- `backend/internal/http/handler/browse.go`
- `backend/internal/service/browse/`
- `docs/superpowers/plans/2026-10-28-115-media-directory-browsing.md`
- `frontend/src/app/router/BrowseRoute.tsx`
- `frontend/src/features/browse/browse-api.ts`

## Newer or mixed frontend edits — inclusion approved

These paths have changed since the recorded implementation or combine task edits with newer changes. Do not silently stage them. Inclusion would preserve the current frontend snapshot, including HeroUI/system theme and auth/connection UI updates.

- `frontend/index.html`
- `frontend/src/app/layouts/AppLayout.tsx`
- `frontend/src/app/providers/AppProviders.tsx`
- `frontend/src/app/router/FoundationPages.tsx`
- `frontend/src/features/auth/LoginPage.tsx`
- `frontend/src/features/auth/auth-login.css`
- `frontend/src/features/connections/ConnectionSidebarCard.tsx`
- `frontend/src/features/connections/ConnectionsPage.ssr.test.tsx`
- `frontend/src/features/connections/ConnectionsPage.tsx`
- `frontend/src/features/connections/connections.css`
- `frontend/src/shared/stores/preferences.ts`
- `frontend/src/styles/globals.css`
- `frontend/public/theme-bootstrap.js`
- `frontend/src/features/browse/BrowsePage.ssr.test.tsx`
- `frontend/src/features/browse/BrowsePage.tsx`
- `frontend/src/features/browse/browse.css`
- `frontend/src/shared/theme/`

## Bookkeeping after approval

1. Commit the confirmed work scope.
2. Archive `10-28-115-media-directory-browsing`.
3. Record the session journal with the work commit hash.

No remote push is planned.

User confirmed inclusion of all listed changes and commit/archive on 2026-10-30.
