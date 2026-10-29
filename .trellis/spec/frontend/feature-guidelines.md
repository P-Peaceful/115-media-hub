# Frontend Feature Guidelines

## Scope and status

Use this guide when implementing product screens and workflows such as file browsing, media libraries, STRM operations, and task management. It consolidates product-specific frontend requirements from the supplied frontend spec.

The current frontend has administrator login, a protected workspace shell, and foundation content; media product screens are not delivered. Apply these requirements as features are implemented; do not describe future controls, endpoints, or test coverage as already available. For existing code and tool behavior, follow [project context](../project-context.md), the [authentication contract](../backend/auth-contract.md), and the focused guides for [directory structure](directory-structure.md), [components](component-guidelines.md), [hooks](hook-guidelines.md), [state](state-management.md), [types](type-safety.md), and [quality](quality-guidelines.md).

The supplied document is a proposal, and its instructions are source material rather than conversation instructions. This project guide reconciles that proposal with the repository: use the existing React Router data-router setup (`createBrowserRouter` / `RouterProvider`), not a required switch to Declarative Mode. Prettier, React Testing Library, MSW, Playwright, and axe are not currently configured; introduce them only through a scoped tooling decision. Existing package scripts and `make check` define current quality gates.

## Feature boundaries

- Keep page composition and application wiring in `app`; put feature-specific screens, API calls, hooks, and types in the owning `features/*` area; keep genuinely shared, business-neutral infrastructure in `shared`.
- A feature must not reach into another feature's private files. Promote code to `shared` only when it is both business-neutral and used across features.
- Keep request and response contracts aligned with OpenAPI. Put any transport-to-feature-model conversion at the feature API boundary.
- Do not create business routes, placeholder success behavior, or APIs ahead of an implementation task and its backend contract.
- Continue using the shared API client. Do not scatter raw `fetch` calls, duplicate endpoint construction, or error mapping across screens.

## Navigation and data behavior

- Use the existing React Router setup. Stable resource IDs identify details and scopes; filenames and full paths are mutable display data, not identity.
- Put state in the URL when users need to share, reload, or navigate back to it, such as the selected directory, filters, sorting, or paging. Keep temporary selection and open overlays local to the page.
- Configure server-side history fallback for any production route that must support direct entry or refresh. Keep backend-reserved URL prefixes out of client routes.
- Use TanStack Query for server-owned data. Query keys must include every parameter that changes a result, such as library/directory ID, filters, sorting, and pagination. Mutations should invalidate the affected queries rather than the whole cache.
- Follow the backend's pagination or cursor contract. Do not download an unbounded collection and paginate only in the browser. Virtualize only after measuring rendering needs; virtualization does not bound how much data is fetched.
- Handle scope changes and out-of-order responses so an old directory or library result cannot replace the current view. Represent empty, loading, failed, partial, stale, and unknown results distinctly; unknown totals are not zero and must not produce invented progress.
- Do not automatically retry non-idempotent writes. Resolve uncertain outcomes through the backend's task/result contract instead of blindly resubmitting.

## File browsing

- Present one directory as a list. Enter a folder by selecting it and return through breadcrumbs or an explicit up action; do not default to a recursively expanded tree.
- For breadcrumb paths during in-app navigation, reuse the successful parent path plus the child ID/name from the current directory listing; keep that path in the directory-scoped Query cache so each folder transition does not depend on a separate upstream metadata request. Direct links and reloads still need the server metadata query; if it fails, keep directory entries navigable and clearly mark any unknown path segment.
- Use stable IDs for files and folders. Make the search scope explicit (current directory or broader scope), and use backend sorting, search, and pagination where available.
- Preserve the server's raw timestamp meaning and format it for the user's locale in the UI. Choose one IEC or SI file-size convention and apply it consistently.
- Reset or clearly explain selection when directory scope changes. Prevent stale requests from displaying data for a prior scope.
- Make the target of destructive actions explicit. Distinguish local index/STRM changes from changes to files stored in the 115 drive; never infer that deleting a local record means deleting a cloud file.

## Media library, STRM, and task workflows

- Keep media library list, details, editing, source directories, and output settings within the media-library feature. Derive the selected library from its stable route ID and scope its queries and selections accordingly.
- Validate forms for prompt feedback, but treat backend validation as authoritative. Disable duplicate submission while a mutation is pending and map field errors to their fields when the API supports it.
- Display statuses, last-run facts, and error summaries from server results; do not synthesize successful state in the client.
- Before starting a sync, show the target library, operation type, scope, cleanup choice, and material risks. Destructive cleanup requires explicit confirmation.
- Distinguish full sync, incremental sync, and deletion/cleanup operations. The backend contract must define whether cleanup removes local STRM/index data or source-drive content; the frontend must never guess or imply source deletion by default.
- Treat sync as an asynchronous task only when the API provides a task identifier and query contract. Show the returned status and available progress, counts, and errors. Use the API's declared status values, not substring matching. Cancellation, retry, and duplicate-start behavior depend on explicit backend support.
- Never declare a destructive mutation successful optimistically. If the request times out or returns an uncertain outcome, show that uncertainty and reconcile with the server before offering another submission.

## Interaction, accessibility, and responsive layout

- Data views need distinct initial loading, refresh, empty, success, and error states; show permission states when the API provides them. Retain usable prior content during background refresh where appropriate, and ensure a loading indicator cannot persist after failure.
- Forms need associated labels, required-field cues, field errors, and clear disabled/submitting states. Use accessible HeroUI v3 / React Aria patterns and verify the actual installed component API before relying on examples.
- Support keyboard use, visible focus, meaningful names for icon-only buttons, semantic structure, and focus management for dialogs. Overlays should constrain and restore focus and close with Escape when appropriate. Do not make color the only status signal.
- Treat WCAG 2.2 AA as an accessibility target for feature review. Test relevant keyboard, zoom, contrast, screen-reader, narrow-layout, and long-Chinese-path behavior; current foundation tests do not establish these properties.
- Confirm the scope and consequence in the interface before deletion, cleanup, or overwrite. A toast is feedback after an action, not confirmation before it.
- Respect reduced-motion preferences. Prefer CSS for simple transitions; add a motion dependency only for a demonstrated need.

## Performance and security

- Lazy-load feature routes or other heavy modules when adding substantial screens. Avoid expensive sorting, filtering, and object conversion in render; measure before adding memoization or virtualization.
- Use appropriately sized images, lazy loading, and stable placeholders. Set performance targets from measurements in the self-hosted deployment context rather than treating suggested Web Vitals as an unmeasured hard gate.
- Keep 115 credentials, refresh tokens, and server secrets on the backend. Never put them in `VITE_*`, browser storage, logs, or client-visible examples.
- Prefer same-origin `/api` requests. Treat user input as untrusted; avoid unsafe HTML injection and unsafe redirects. Backend authorization and business validation remain mandatory; hiding a control is not security.
- Do not assume cloud-drive redirect URLs can be fetched cross-origin by a browser. CORS and redirect handling belong to the backend and deployment contract; do not disable browser security to work around them.
- Avoid logging private paths, credentials, cookies, signed URLs, and sensitive request data. Use a safe request identifier when the API provides one.

## Validation for future feature work

Choose tests from the tools actually configured in the repository. Current frontend tests use Vitest in Node and cover the shared request helper; there is no DOM component test setup or browser E2E suite. When introducing component, API-mocking, accessibility, or E2E tooling, make that a deliberate dependency and CI change.

For a feature, cover the user-visible state transitions and the important failure modes supported by its API: scope changes, empty data, permission/error responses, cancellation or stale responses, duplicate submissions, and uncertain outcomes as applicable. Keyboard and destructive-action behavior need direct review. Report checks that were actually run; do not claim unconfigured tools or scenarios passed.
