# Database Guidelines

## SQLite and ownership

Architecture comes from the [baseline](../guides/project-baseline.md). GORM access belongs in target `internal/repository/sqlite/`; services own use-case transaction boundaries through repository abstractions. Do not pass `*gorm.DB` into HTTP handlers or domain objects.

Use the CGO SQLite driver selected in Notion. Enable WAL, foreign keys, and a finite busy timeout. Set foreign keys and busy timeout for every pooled connection using driver connection options; executing a PRAGMA once does not configure future connections. Verify settings on real connections. Start with one open database connection to serialize local writes; increase only with measured contention tests and per-connection configuration. A transaction must use its transaction handle exclusively to avoid deadlocking a one-connection pool.

Use `db.WithContext(ctx)`, bind values as parameters, allowlist dynamic sort/column names, and bound result sets. Check both errors and `RowsAffected`. GORM default scopes and zero-value updates must be intentional: use explicit column updates/maps when persisting false, zero, or an empty value. Avoid automatic soft deletion unless the feature defines its effect on uniqueness, sync, and cleanup.

## Schema and migrations

- Use goose versioned SQL in target `migrations/`, reusing the underlying `*sql.DB`. Run migrations once before readiness/scheduling/workers; fail startup on migration failure.
- Production schema changes use migrations; do not call GORM `AutoMigrate` on application startup.
- Use unique increasing migration versions and descriptive names. Never change an already applied migration; add a new one. Test a fresh database and upgrade from the prior schema.
- Use snake_case plural table names and snake_case columns; name constraints/indexes explicitly. Add foreign keys, uniqueness, and checks for invariants the database can enforce. Index measured access paths, including job claim predicates.
- Preserve external identifiers as strings. Keep API identifiers independent of storage representation. Persist instants consistently in UTC and test GORM/SQL/API round trips.
- A Down migration must either be tested and safe or explicitly reject an irreversible operation. Do not pretend data restoration is achieved by recreating empty columns. Back up before destructive migrations; rollback may require the matching prior binary plus a consistent backup.

## Transactions and durable effects

Keep transactions short; defer network calls, file writes, and long scans. The service decides the atomic unit and repository code implements it. Persist intended effects, perform external work, then record verified results; handle an interruption between any pair of those steps. Never hold a write transaction while waiting for an upstream call.

Claim jobs with a conditional update and accept ownership only after verifying the result. Illustrative repository fragment (target API/model names must be defined by the job implementation):

```go
result := db.WithContext(ctx).Model(&JobModel{}).
    Where("id = ? AND status = ?", jobID, "queued").
    Updates(map[string]any{"status": "running", "started_at": now})
if result.Error != nil {
    return false, fmt.Errorf("claim job: %w", result.Error)
}
return result.RowsAffected == 1, nil
```

This fragment only demonstrates a conditional transition. The complete job design must define attempts/ownership, resource locks, cancellation, scheduling order, bounded retries, and restart reconciliation. Do not equate a successful claim with exactly-once external execution.

## Required checks

Use real temporary-file SQLite databases to test WAL, foreign keys on pooled connections, rollback, duplicate claims, zero-value updates, upgrades, and relevant query behavior. Do not substitute mocks for SQLite semantics. Test incomplete directory scans never causing deletion, ownership/hash conflicts, and uncertain upload recovery when those features are implemented.

Backups must use a SQLite-consistent mechanism; copying only the main database file while WAL writes continue is unsafe. Keep database files on local disk and reconcile file state after restore before cleanup/overwrites resume.

References: [GORM transactions](https://gorm.io/docs/transactions.html), [SQLite WAL](https://sqlite.org/wal.html), [goose provider](https://pressly.github.io/goose/documentation/provider/).
