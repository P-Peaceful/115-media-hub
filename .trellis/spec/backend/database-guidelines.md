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

### Design decision: application-managed relationships and validation

The user decided on 2026-10-24 that business schemas must omit database foreign keys (`REFERENCES`, `FOREIGN KEY`, `ON DELETE`) and `CHECK` constraints. Preserve relationship ID columns, query indexes, `PRIMARY KEY`, `UNIQUE` (including partial indexes), `NOT NULL` and defaults. See the [schema design](../../../docs/database-design.md) and [SQL draft](../../../docs/database-schema.sql); these remain design artifacts, not applied migrations. The existing `foreign_keys=on` connection setting stays unchanged and does not create constraints on tables that declare none.

Services must validate referenced-object existence, cross-library ownership, enums, booleans, numeric ranges, JSON shape/version, singletons and cross-field/state invariants. Validate and write within one short transaction; check the values that control partial unique-index predicates before persisting them. This single-person utility uses physical deletion rather than soft-delete columns. Deletion code must check durable references, clear rebuildable dependent cache rows, and retain required run facts in immutable snapshots as specified in the design dictionary. Backup restore, import, migration and maintenance writers must obey the same contracts; direct SQL does not enforce them.

### Design decision: single-person utility, compact persistence

On 2026-10-25 the user explicitly rejected database administrator/user/session modeling and requested a simpler tool. The compact draft has 17 tables / 226 fields after the user requested removal of object revision and soft-delete columns, superseding the previous 39-table design. Do not add administrator/session/role tables or user actor foreign identifiers. A single configured management password and in-memory sessions are sufficient; restart/password changes invalidate sessions. This is a future design, not an implemented authentication feature.

Keep global defaults/secrets in configuration, transient health/locks/playback resolution in memory, images and separately rotated logs/audit on disk, backup metadata in manifests, and the durable restore gate in an atomic state file outside the restored DB. Merge connection credentials/capabilities into `connections`, current rules/bindings into `libraries`, scan batch scope into `job_runs`, preview details into `job_items`, and current companion file observations/baselines into `metadata_files`. Use typed/versioned JSON for bounded per-object payloads; retain rows for unbounded scan entries and task items. Preserve `job_effects`, stable asset/output ownership, per-directory completeness, metadata decisions and quarantine facts. Single-person use does not remove restart or external-write hazards.

Do not add object revision or soft-delete columns to the database draft. Use config hashes and immutable run snapshots where historical context is needed; use enabled/is_active to disable current configuration and physically delete only after service-level dependency checks. Resource exclusion is process-local under the single-instance contract. Startup must reconcile unfinished effects before new conflicting writes; adding another application process requires a new concurrency design. Restoring an old DB must not clear the external restore gate. Test these behaviors when implementing the services; the SQL draft alone cannot enforce them.

The driver is `gorm.io/driver/sqlite` / `mattn/go-sqlite3`; [Makefile](../../../Makefile) and [Dockerfile](../../../deploy/Dockerfile) build with `CGO_ENABLED=1`. Preserve the compatible runtime. Do not move the DB onto a remote share or assume an in-memory DB has equivalent WAL behavior.

## 4. Validation & Error Matrix

| Condition | Current behavior |
| --- | --- |
| New nested local path | Create directories/file and migrate |
| Existing migrated path | Reopen and apply only outstanding migrations |
| Path creation or open fails | Return error; caller must stop startup |
| SQLite open fails | Return wrapped `open SQLite: %w` |
| goose initialization/Up fails | Close SQL connection; return `migrate SQLite: %w` |

Future business validation: missing references, invalid ownership/value/state or a forbidden deletion must return a classified service error with no partial writes. A unique-key collision remains a database constraint failure handled by the repository/service. Business implementations and error mappings are not delivered by the design draft.

## 5. Good/Base/Bad Cases

Good: reopen a real temporary file and verify migration version and PRAGMAs. Base: startup creates only goose bookkeeping. Bad: use `AutoMigrate`, raise pool size as a lock workaround, or retain a transaction across 115/network/filesystem work; each bypasses a deliberate safety boundary.

Future transactions belong to services, stay short, and persist effects separately from external I/O. The [functional requirements](https://app.notion.com/p/401138389edc81298a1aeb6012d4f8e8) require persistent plans/runs/items/effects, conditional claims with `RowsAffected`, bounded retries and verified restart recovery. No claim/retry SQL exists to copy yet. A lock is not protection against external writers; uncertain uploads require reconciliation.

## 6. Tests Required

[database_test.go](../../../backend/internal/database/database_test.go) opens twice and asserts `journal_mode=wal`, `foreign_keys=1`, `busy_timeout=5000`, applied version `1` and pool maximum `1`. Preserve those assertions; new migrations need real SQLite upgrade tests, constraint tests and rollback/failure assertions appropriate to their schema. [smoke.py](../../../scripts/smoke.py) checks absence of business tables in the foundation. Run `make check`; use `make smoke` for packaging/startup changes.

For future business migrations, assert all `PRAGMA foreign_key_list(table)` results are empty and table definitions contain no `CHECK`. Verify retained uniqueness/non-null behavior separately. Service tests must reject invalid IDs, enum/range/JSON/state combinations and unsafe deletions before writing, and verify transaction rollback and explicit dependent-row handling. Passing schema introspection is not evidence that code-side validation exists.

## 7. Wrong vs Correct

Wrong: `db.AutoMigrate(...)` at startup. Correct: a reviewed numbered SQL migration embedded by the existing provider. Wrong: perform an upload inside a GORM transaction. Correct (future implementation): perform bounded external work outside the short transaction and persist verified effects, so SQLite locks do not span network delays.

Wrong: add `CHECK (enabled IN (0,1))` or `REFERENCES libraries(id)` to a new business migration. Correct: declare ordinary ID/value columns with the retained structural constraints, and validate those business rules in the service transaction. Wrong: delete a referenced library with raw repository SQL. Correct: keep its soft-delete marker or reject deletion while durable references remain.
