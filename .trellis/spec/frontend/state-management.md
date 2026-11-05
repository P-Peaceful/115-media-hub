# State Management

## One owner per state

| Category | Owner | Examples |
| --- | --- | --- |
| Server data | TanStack Query | Directory entries, connections, libraries, jobs, media, system settings |
| Navigable state | React Router URL/search params | Directory ID, library detail, filters, sort, pagination cursor |
| Local interaction | Component state/reducer | Open dialog, draft form, transient selection |
| Shared UI preference | Narrow Zustand store | Theme, sidebar/layout preference |
| Derived values | Compute from existing state | Counts, selected labels, filtered view of loaded data |

Do not copy query responses into Zustand or component state for convenience. A form may own an editable draft with explicit reset/save behavior, but must not silently overwrite edits on refetch. Store backend business settings through the API even if they look like UI preferences.

## Routing and browsing

Use the single-directory list plus breadcrumbs/back navigation selected in Notion. Keep navigable directory/filter/page state in the URL so refresh and browser history work. Validate URL params, reset cursor on filter/sort/directory changes, and treat cursors as opaque. Do not keep a conflicting URL copy in Zustand.

Virtualization only reduces rendering cost for already loaded rows. Use pagination or cursor loading and stable identifiers; bound retained pages/selections where lists can grow indefinitely.

## Preferences and sessions

Use small Zustand stores with focused selectors; persist only explicitly chosen non-sensitive preferences under versioned keys with migration/reset behavior. Never persist passwords, cookies, session/CSRF tokens, external credentials, temporary direct links, or full query responses in localStorage.

Admin sessions use backend cookies. Query session status through the API; do not invent a second authentication truth in a persisted boolean. On logout/account/session changes, close SSE, clear protected query caches, and reset transient protected state. Prevent an old request/event from repopulating data after logout.

## SSE consistency

Durable state is owned by the server/database and cached by Query. SSE notifications trigger targeted cache updates or invalidation. On reconnect refetch subscribed state, since the initial event contract does not provide durable replay. Handle malformed/unknown events safely, release subscribers on unmount, and avoid one stream per component.

Review using the [shared event contract](../guides/project-baseline.md#authentication-and-event-boundaries) and [hook guidance](./hook-guidelines.md).
