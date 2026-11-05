# Backend Quality Guidelines

## Style and readability

Use [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) as the language baseline. Apply `gofmt` (tabs in Go), and `goimports` for import grouping: standard library, third party, local packages. Pin formatter/linter versions during initialization.

Use early returns, short cohesive functions, explicit dependency construction, and intention-revealing names. Exported symbols have useful Go doc comments. Avoid stutter (`library.LibraryService`), naked returns in nontrivial functions, speculative interfaces, boolean-mode functions with unrelated behaviors, and package-level mutable state. Comment on invariants and reasons rather than narrating assignments.

Accept `context.Context` as the first argument for operations that can block; propagate cancellation/timeouts and close bodies/rows/files. Document ownership/lifecycle of goroutines and channels. Do not store request contexts in long-lived services or spawn unbounded goroutines. Keep domain code free from Gin/GORM and enforce the [shared contract](../guides/project-baseline.md).

## Future tooling gate

No Go module or application CI exists yet. The initialization task must pin Go, `goimports`, and a compatible `golangci-lint`, configure lint checks for unchecked errors, static analysis, vet, and ineffectual assignments, and make these checks reproducible in local scripts/CI:

```text
# From the repository root, once go.mod and Go source exist:
go vet ./...
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
golangci-lint run ./...
```

Formatting verification must fail on diffs from `gofmt`/`goimports`, not merely run a formatter and succeed. Run the race gate on a supported CGO toolchain. Build and smoke-test the final embedded frontend/CGO Docker image as part of integration delivery. Document any environment limitation instead of treating a skipped check as passing.

Do not ignore errors with `_`, blanket-disable lint, or add sleeps to hide races. A narrow suppression needs a reason adjacent to it. Generated files are excluded from manual style edits but must regenerate deterministically and compile.

## Meaningful tests

- Use the standard Go test framework, table-driven cases when useful, and temporary resources with cleanup. Test observable behavior; avoid assertions that simply repeat implementation expressions.
- Service/domain tests cover invariants, permissions, and errors. Use small fakes at external boundaries; repository tests use real SQLite for SQL semantics.
- HTTP tests use `httptest` and cover validation, status/envelope mapping, session/CSRF behavior, and SSE lifecycle when implemented.
- Job tests cover concurrent claims, duplicate submissions/effects, cancellation, retry bounds, restart verification, and resource locking. Prefer controlled clocks/signals over timing sleeps.
- File/sync tests cover ownership hashes, incomplete scans, quarantine paths, and uncertain uploads before shipping those capabilities.
- For each business module, test necessary logging at the owning operation boundary: success/failure and applicable cancellation/conflict/recovery, required structured attributes, correlation, redaction, and no duplicate terminal errors. Verify documented aggregation for high-volume reads and complete critical audit records.
- Add a regression for a fixed behavioral bug. Documentation-only changes do not require application unit tests.

## Review gate

Verify relevant backend guides, the frontend contract, cancellation/resource cleanup, and Notion's file/data safety invariants. Every implemented business module must satisfy the [required logging coverage](./logging-guidelines.md#required-business-module-coverage), with an identified owner for key actions, state changes, and terminal outcomes. HTTP access logs alone are insufficient. Keep filesystem/network I/O out of transactions. Check changed migrations on fresh and prior databases. Record actual commands/results, generated contract changes, and material limitations. Never claim lint/tests passed before initialization makes them runnable.
