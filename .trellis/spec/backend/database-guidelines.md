# Database Guidelines

## 1. Scope / Trigger

Read this before changing persistence, migration or deployment storage. [Project context](../project-context.md) and [backend architecture](https://app.notion.com/p/401138389edc811fabfcefbf05f26845) require one instance with SQLite on local disk. Business repositories and tables are absent.

## 2. Signatures

[database.Open](../../../backend/internal/database/database.go) exposes:

```go
func Open(path string) (*gorm.DB, error)
```

It creates the parent directory with `0700` and the file with `0600` when absent, opens GORM with translated errors and silent SQL logging, obtains the underlying `*sql.DB` and applies embedded goose migrations. Creation modes do not repair permissions on existing paths.

## 3. Contracts

The SQLite URI escapes the absolute path through `url.URL`, preserving special characters. Existing code sets:

```go
q.Set("_journal_mode", "WAL")
q.Set("_foreign_keys", "on")
q.Set("_busy_timeout", "5000")
sqlDB.SetMaxOpenConns(1)
sqlDB.SetMaxIdleConns(1)
```

Reuse that SQL connection for goose. [Embedded migrations](../../../backend/migrations/embed.go) load numbered SQL; [00001_baseline.sql](../../../backend/migrations/00001_baseline.sql) contains `-- +goose Up` / `Down` and `SELECT 1`, creating no business tables. Add versioned SQL rather than `AutoMigrate`; test upgrades and deliberate rollback behavior. No business table/column naming precedent exists yet: decide it explicitly in the first schema task.

The driver is `gorm.io/driver/sqlite` / `mattn/go-sqlite3`; [Makefile](../../../Makefile) and [Dockerfile](../../../deploy/Dockerfile) build with `CGO_ENABLED=1`. Preserve the compatible runtime. Do not move the DB onto a remote share or assume an in-memory DB has equivalent WAL behavior.

## 4. Validation & Error Matrix

| Condition | Current behavior |
| --- | --- |
| New nested local path | Create directories/file and migrate |
| Existing migrated path | Reopen and apply only outstanding migrations |
| Path creation or open fails | Return error; caller must stop startup |
| SQLite open fails | Return wrapped `open SQLite: %w` |
| goose initialization/Up fails | Close SQL connection; return `migrate SQLite: %w` |

## 5. Good/Base/Bad Cases

Good: reopen a real temporary file and verify migration version and PRAGMAs. Base: startup creates only goose bookkeeping. Bad: use `AutoMigrate`, raise pool size as a lock workaround, or retain a transaction across 115/network/filesystem work; each bypasses a deliberate safety boundary.

Future transactions belong to services, stay short, and persist effects separately from external I/O. The [functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8) require persistent plans/runs/items/effects, conditional claims with `RowsAffected`, bounded retries and verified restart recovery. No claim/retry SQL exists to copy yet. A lock is not protection against external writers; uncertain uploads require reconciliation.

## 6. Tests Required

[database_test.go](../../../backend/internal/database/database_test.go) opens twice and asserts `journal_mode=wal`, `foreign_keys=1`, `busy_timeout=5000`, applied version `1` and pool maximum `1`. Preserve those assertions; new migrations need real SQLite upgrade tests, constraint tests and rollback/failure assertions appropriate to their schema. [smoke.py](../../../scripts/smoke.py) checks absence of business tables in the foundation. Run `make check`; use `make smoke` for packaging/startup changes.

## 7. Wrong vs Correct

Wrong: `db.AutoMigrate(...)` at startup. Correct: a reviewed numbered SQL migration embedded by the existing provider. Wrong: perform an upload inside a GORM transaction. Correct (future implementation): perform bounded external work outside the short transaction and persist verified effects, so SQLite locks do not span network delays.
