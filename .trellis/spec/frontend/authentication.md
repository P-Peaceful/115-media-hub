# Authentication UI Contract

## 1. Scope / trigger

`frontend/src/features/auth/` owns first-run setup, login, session lifecycle and local-recovery guidance. `src/app/app.tsx` composes routing/providers. Read this contract when changing authentication, authenticated API consumption or protected cache behavior, together with the [backend contract](../backend/authentication.md) and root OpenAPI.

## 2. Signatures

Routes: `/setup`, `/login` and protected `/`. The authenticated home composes the application shell in `src/app/` with the independent `features/dashboard/` page and logout; its system status combines public readiness with authenticated process information; see the [system status contract](../backend/system-status.md).

`features/auth/api.ts` derives `AuthBootstrap`, `AuthSession`, `LoginRequest` and `InitializeRequest` from generated `components`. `decodeBootstrap`/`decodeSession` validate network responses before hooks/components consume them. `AuthProvider`/`useAuth` own server session state and transitions. `safeReturnPath` owns validation of a local post-login destination. `lib/api/client.ts` owns JSON requests, safe errors and protected-session expiry notification.

## 3. Contracts

The Vite development proxy must use explicit `{target, changeOrigin: false}` objects for `/api` and `/health`. In pinned Vite, a string target expands to `changeOrigin: true`, causing browser Origin/upstream Host disagreement and strict CSRF rejection. Keep backend origin validation strict; verify actual setup through the real proxy, not only mocked API routes.

Use TanStack Query bootstrap state as authentication truth; no persisted authenticated boolean. Bootstrap includes explicit nullable identity/expiry. Keep CSRF only in memory; use same-origin credentials for requests. Do not include passwords/tokens in query keys, URL parameters, router state, analytics or browser storage.

After login/setup, replace the bootstrap state with the returned session and clear protected state. Logout, server 401, revocation or expiry cancels/removes protected requests/cache before unauthenticated rendering. Auth transitions must prevent old in-flight protected requests from repopulating the prior account's cache. Recheck on focus/reconnect; use a bounded expiry timer that cannot overflow for 30-day sessions. Do not automatically retry credential mutations.

Login fields: username/password and remember-login. Setup adds password confirmation, lifetime choices 1/7/30 (default 7), service URL (current origin) and output directory `/app/media`. The output is a server absolute path entered as text. Validate passwords as 8–32 printable non-space ASCII characters U+0021–U+007E, rejecting Chinese/non-ASCII, spaces and controls without trimming or adding a mixed-class requirement; usernames as case-sensitive 3–32 ASCII letters/digits/underscore/period/hyphen. Validate service URLs and paths consistently with the backend; server validation remains authoritative.

Use supported HeroUI 3.2.7 components for applicable inputs, forms, labels, cards, checkbox, select, buttons, dialogs, links and feedback. Consult the actual v3 documentation first; React/HeroUI legacy props or deprecated React event types are not valid substitutes. Semantic HTML/CSS owns layout/content and the bundled illustration owns decoration.

The user's design adjustments are part of acceptance: remove the entire top header/status row and 115 authorization card/entry. Preserve main login branding, purple gradients, panel geometry, typography and media-folder illustration. Do not render design artboard captions. A standalone lower-right theme toggle uses HeroUI's documented `useTheme("system")`, allowing persisted light/dark choice and system preference. Theme CSS follows the root `.light`/`.dark` and `data-theme` attributes; HeroUI controls use semantic theme tokens. Maintain keyboard/focus behavior and reduced motion. Reserve space for decoration so mobile text is never obscured.

Forgotten-password displays the real local maintenance command, explains stopping the application and using the original database, and links README recovery instructions. Container guidance must support hidden TTY input or explicitly selected `--password-stdin`; it must not imply that email recovery exists.

## 4. Validation and error matrix

| Condition | UI behavior |
| --- | --- |
| Bootstrap pending | Accessible loading state; no protected-page flash |
| Bootstrap offline/invalid response | Retry state; no assumed initialized/authenticated result |
| Uninitialized | Route to setup |
| Initialized but anonymous | Route to login with validated local return path |
| Authenticated visit to login/setup | Route to safe requested local destination or home |
| Invalid fields/password mismatch | Associated field messages; no duplicate mutation |
| Invalid credentials | Safe common username/password message |
| CSRF/setup race error | Refresh bootstrap state; keep mutation outcome explicit |
| Logout/expiry/protected 401 | Clear protected state and return to login |
| Malformed success/error payload | Safe invalid-response handling; never unchecked DTO casting |

JavaScript `$` alone can match before a final newline. Validate the ASCII range and length explicitly (or use a true-end pattern), and test terminal LF/CR,33-character paste and non-ASCII without trimming/truncation.

Keep request IDs bounded and safe when displayed. Do not display internal database/SDK errors or treat missing data as a successful empty response.

## 5. Good/base/bad cases

- Good: fresh install → setup → authenticated home → logout → login → local return path, with cookies handled by the browser.
- Base: a valid remembered session survives refresh/restart; server expiry still forces reauthentication.
- Bad: external/protocol-relative/backslash/control-character return destinations, a folder illustration covering mobile branding, a checkbox label detached from its glyph, or tokens saved in localStorage.

## 6. Tests required

Unit tests cover runtime payload invariants, safe return destinations, form constraints and ASCII password length/character bounds (including trailing newline rejection). Component tests cover pending/failure/success/expiry, recovery-dialog focus, password visibility, initialization fields and logout/cache transitions. Browser tests compare desktop/mobile light/dark screenshots, keyboard form submission, no horizontal overflow, and absence of removed header/authorization UI.

Mocked tests may run concurrently; real-backend setup mutations need isolation. `scripts/test-integration.py` runs desktop and mobile-dark sequentially with a separate temporary backend/database for each project; each project keeps real initialization/login/logout in one intentionally ordered workflow. Update the harness if adding another browser project.

Visual review must compare typography, spacing, illustration, gradients, shadows and field/button alignment against the corresponding adjusted reference artboard. Correct material differences; behavior-test success alone is insufficient.

## 7. Wrong vs correct

Wrong: copy v2 HeroUI props into v3 controls, derive auth from a persisted boolean, build redirects directly from query input, use a string shorthand for the Vite proxy, or overlay an absolute illustration over text at narrow widths.

Correct: verify the pinned component's compound API, decode cookie-backed bootstrap through the feature API boundary, validate destinations in one owner, and give artwork explicit responsive layout space. Keep policy/state orchestration separate from focused presentation components.
