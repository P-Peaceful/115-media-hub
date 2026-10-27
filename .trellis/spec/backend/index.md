# Backend Development Guidelines

These English guidelines describe the Go foundation as reviewed on 2026-10-23. Read [source authority and scope](../project-context.md) first: local code proves current behavior; Notion documents constrain future work.

| Guide | Use |
| --- | --- |
| [Go coding](go-coding-guidelines.md) | Naming, boundaries, errors, context, API safety, security, testing and AI-assisted changes |
| [Directory structure](directory-structure.md) | Composition, package boundaries and API ownership |
| [Database](database-guidelines.md) | SQLite, migrations and future transaction rules |
| [Error handling](error-handling.md) | Startup configuration, health responses and SPA failures |
| [Administrator authentication](auth-contract.md) | Required deployment credentials, 24-hour sessions, Cookie/CSRF and management API boundary |
| [115 connection contract](connection-guidelines.md) | QR lifecycle, server-side credential persistence, connection status, API and failure handling |
| [Logging](logging-guidelines.md) | Structured events, rotation and privacy |
| [Quality](quality-guidelines.md) | Checks, review boundaries and future acceptance tests |

## Pre-Development Checklist

1. Read the active task PRD, [project context](../project-context.md), and [shared thinking guides](../guides/index.md).
2. Read Go coding, directory structure and quality; then database for persistence, error handling for configuration/HTTP, and logging for any new event.
3. Inspect [composition](../../../backend/internal/app/app.go), [OpenAPI](../../../api/openapi.yaml), and actual implementation before proposing a business pattern. Most business directories contain only `.gitkeep`; authentication is implemented.
4. For cross-layer changes identify API fields, generated consumers, state ownership and failure assertions before editing. For commands use the root [Makefile](../../../Makefile).

## Quality Check

Run from the repository root:

```sh
make check
make generate  # when OpenAPI changes; review both generated outputs
make smoke     # when startup, build, HTTP or embedded UI behavior changes
```

`make check` covers formatting, Go vet/tests and frontend checks. `make smoke` builds the real binary with CGO and embedded UI; it is broader than unit tests. See [development notes](../../../docs/development.md) for configuration and local development. Documentation-only changes require link/example review; do not claim a check ran without its output.
