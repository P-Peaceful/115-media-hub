# Backend Development Guidelines

## Scope and authority

Read the [project baseline](../guides/project-baseline.md) first for Notion sources, architecture constraints, shared API decisions, and current implementation status. The Go modular monolith is rooted in `backend/`; startup, SQLite migrations and health HTTP behavior are implemented. Feature snippets without source links describe future business modules.

## Guidelines

| Guide | Responsibility |
| --- | --- |
| [Initialization](./initialization.md) | Implemented startup, health, generated contracts and build boundaries |
| [Directory structure](./directory-structure.md) | Package boundaries, dependency direction, naming |
| [Database](./database-guidelines.md) | SQLite configuration, repositories, migrations, task claims |
| [Error handling](./error-handling.md) | Error propagation, classification, HTTP mapping |
| [Logging](./logging-guidelines.md) | slog, rotation, audit, redaction |
| [Quality](./quality-guidelines.md) | Idiomatic Go, tooling, tests, review |

## Pre-Development Checklist

1. Read the current task's requirements and the project baseline.
2. Read directory structure, logging, and quality for every backend change; every business module needs the logging coverage defined in the logging guide.
3. Read database for persistence/jobs; error handling for service/HTTP changes; the logging audit/redaction sections for credentials or critical operations.
4. For cross-layer changes, read the [cross-layer guide](../guides/cross-layer-thinking-guide.md) and update the OpenAPI contract before consumers.
5. Search the implementation before adding a package or abstraction; add real source references to these specs as code lands.

## Quality Check

Follow [quality gates](./quality-guidelines.md). Verify relevant Notion invariants and the frontend contract, not only formatting. State which checks actually ran. `backend/Makefile` owns backend commands and `.github/workflows/ci.yml` invokes the cross-project gates.
