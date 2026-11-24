# Backend Quality Guidelines

## Style and readability

Use [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) as the language baseline. Apply `gofmt` (tabs in Go), and `goimports` for import grouping: standard library, third party, local packages. Pin formatter/linter versions during initialization.

Use only supported, non-deprecated APIs in new or changed backend code. Check official documentation for the pinned Go/dependency version and relevant source deprecation annotations; successful compilation alone is insufficient. Keep transport, use-case policy and persistence focused on their own responsibilities.

Use early returns, short cohesive functions, explicit dependency construction, and intention-revealing names. Exported symbols have useful Go doc comments. Avoid stutter (`library.LibraryService`), naked returns in nontrivial functions, speculative interfaces, boolean-mode functions with unrelated behaviors, and package-level mutable state. Comment on invariants and reasons rather than narrating assignments.

Accept `context.Context` as the first argument for operations that can block; propagate cancellation/timeouts and close bodies/rows/files. Document ownership/lifecycle of goroutines and channels. Do not store request contexts in long-lived services or spawn unbounded goroutines. Keep domain code free from Gin/GORM and enforce the [shared contract](../guides/project-baseline.md).

## Required Chinese comments

This is an ongoing project requirement for all handwritten backend Go files, including production code, tests, entry points, migrations/embed declarations, and build variants. Apply it when creating or changing a file; the separate backend-comment task handles the existing-file backlog. English identifiers and spec prose remain unchanged by this rule.

- **File responsibility:** every handwritten `.go` file must start with a Chinese description of that specific file's main responsibility and relevant boundaries. A package name or a generic comment such as “处理业务逻辑” is insufficient. Keep the description near the beginning, before `package`, after any required license/build constraints. Explain the file's role even when another file already documents the package.
- **Function/method responsibility:** every exported function/method and every significant private function/method must have a Chinese doc comment. Significant private functions include use-case orchestration, validation, protocol parsing, database mutations, concurrency control, cancellation, filesystem publication/recovery, and security checks. Explain what the method does and, where relevant, its input constraints, return/error meaning, side effects, resource ownership, and invariants. Keep the Go identifier at the beginning of a declaration's doc comment. Do not require repetitive prose for trivial local plumbing without meaningful behavior.
- **Tests and declarations:** test files describe the tested feature and coverage boundaries; test functions explain the scenario or regression being checked. Document important test helpers, types, interfaces, constants, and fields when their contracts or ownership are not obvious.
- **Accuracy and maintenance:** update comments whenever the responsibility or behavior changes. Describe implemented behavior and reasons, not future plans or line-by-line assignments. Do not claim absent retries, encryption, validation, or reliability guarantees. Do not include credentials or real signed URLs in examples.
- **Go directives and package documentation:** preserve `//go:build`, `//go:embed`, `//go:generate`, other tool directives, their placement, and required blank lines. Keep file-specific comments distinct from the package's canonical `Package <name>` documentation; do not copy the same package summary into every file. Go syntax, identifiers, directive text, and required generated/license notices retain their required form.
- **Generated sources:** never hand-edit generated `.go` files to satisfy this requirement. Preserve their generation notices and deterministic output; if generated comments need to change, change the owning schema/template/generator as a separately justified change.

Example file and method comments, using the current source-ID validation contract:

```go
// scan.go：负责遍历媒体库源目录、校验路径与分页结果，并收集可生成 STRM 的视频。
// 扫描只读网盘；不完整目录的结果不能作为完整清单使用。

package strm

import "strings"

// providerID 判断源 ID 是否为 1–32 位十进制数字，供源条目和目录身份校验使用。
func providerID(id string) bool {
	return len(id) > 0 && len(id) <= 32 && strings.Trim(id, "0123456789") == ""
}
```

For a build-specific file, the build constraint stays above the file description:

```go
//go:build production

// production.go：嵌入生产前端资源，并向 HTTP 层提供可读取的资源文件系统。

package web
```

Review must reject a new/changed handwritten file with a missing or generic file description, an undocumented major method, or a comment contradicting behavior. For example, “执行扫描” does not explain why a partial directory is discarded; the owning scan method should describe its completeness and failure boundary. Verify coverage and meaning by reading the implementation; a comment-count check alone is insufficient. Documentation-only changes need consistency, whitespace, and directive checks, not new application unit tests. When a change is claimed to be comment-only, verify executable declarations and behavior remain unchanged.

## Tooling gate

`backend/go.mod`, `backend/Makefile` and lint configuration pin Go and compatible formatter/linter tooling. Go is pinned to 1.26.9, goimports to v0.43.0 and golangci-lint to v2.15.0. Keep checks for unchecked errors, static analysis, vet and ineffectual assignments enabled and reproducible in local scripts/CI:

```text
# From backend/:
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
- File/sync tests cover ownership and content checks, incomplete scans, cleanup of eligible outputs, preservation of modified and unowned files, and uncertain uploads before shipping those capabilities.
- For each business module, test necessary logging at the owning operation boundary: success/failure and applicable cancellation/conflict/recovery, required structured attributes, correlation, redaction, and no duplicate terminal errors. Verify documented aggregation for high-volume reads and complete critical audit records.
- Add a regression for a fixed behavioral bug. Documentation-only changes do not require application unit tests.

## Review gate

Verify the [required Chinese comments](#required-chinese-comments) on every new/changed handwritten Go file, alongside relevant backend guides, the frontend contract, cancellation/resource cleanup, and file/data safety invariants in the [project baseline](../guides/project-baseline.md#v10-boundaries-and-invariants). Every implemented business module must satisfy the [required logging coverage](./logging-guidelines.md#required-business-module-coverage), with an identified owner for key actions, state changes, and terminal outcomes. HTTP access logs alone are insufficient. Keep filesystem/network I/O out of transactions. Check changed migrations on fresh and prior databases. Record actual commands/results, generated contract changes, and material limitations. Never claim lint/tests passed before initialization makes them runnable.
