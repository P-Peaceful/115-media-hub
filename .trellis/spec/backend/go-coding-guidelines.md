# Go Backend Coding Guidelines

## 1. Scope and authority

Use this guide when adding or changing Go code. It consolidates the general Go engineering rules supplied in `GO_BACKEND_SPEC.md` and adapts them to this repository.

Apply Go's idioms, `gofmt`, and the repository's established patterns first. The repository-specific guides in this directory define actual architecture and behavior: [directory structure](directory-structure.md), [database](database-guidelines.md), [error handling](error-handling.md), [logging](logging-guidelines.md), and [quality](quality-guidelines.md). This document does not turn proposed or future architecture into implemented behavior. Avoid unrelated refactors and new dependencies; generated files must be updated through their generator.

The source spec describes Gin + GORM as a default stack. This repository currently uses Gin, GORM and SQLite, but its business service/repository layers are not yet implemented. Follow the current code and the more specific guides before applying proposed patterns.

## 2. Naming and formatting

- Format Go source with `gofmt`; maintain imports with `goimports` or equivalent when available.
- Use short, lowercase package names without underscores. Avoid vague package names such as `common`, `util`, or `helpers`.
- Use PascalCase for exported identifiers and camelCase for unexported identifiers. Use lowercase filenames with underscores when needed (for example, `media_service.go`).
- Keep initialisms consistent: `ID`, `URL`, `HTTP`, `API`, and `JSON` (for example, `userID` and `HTTPClient`).
- Keep receiver names short and type-related; do not use `this` or `self`.
- Name getters `Name()` rather than `GetName()` unless `Get` has specific domain meaning.
- Use Go doc comments for exported APIs, beginning with the identifier name and explaining purpose or rationale.
- There is no fixed line-width, function-length, or parameter-count limit. Split complex code when it improves readability.

## 3. Boundaries and dependencies

The repository's current composition and package ownership are documented in [directory structure](directory-structure.md). Keep dependencies one-way and avoid import cycles. Add packages and interfaces only when there is a real ownership, substitution, or testing need; do not create an interface for every struct.

For future business code, keep the HTTP handler thin: bind and validate input, call the use case, and map its result to HTTP. Business logic must not depend on Gin or return transport objects such as `gin.H` or HTTP status codes. Keep database access behind repository boundaries when those layers are introduced. Use `context.Context` in business and I/O APIs; keep `*gin.Context` at the HTTP boundary. Separate transport DTOs, domain values, and persistence models when their meaning or lifecycle differs. Do not use database models indiscriminately as public request and response types.

Do not add a second route or hand-edit generated API code. Public API changes start in `api/openapi.yaml` and follow the generation process in the repository's quality guide.

## 4. Functions, errors, and logs

- Keep functions focused, prefer early returns, and avoid unnecessary nesting.
- Return errors for expected failures. Do not use `panic` for ordinary business or I/O errors; reserve it for unrecoverable programming faults.
- Handle returned errors. If an error is intentionally ignored, document why.
- Wrap errors that need context with `fmt.Errorf("...: %w", err)` and inspect wrapped errors with `errors.Is` or `errors.As`.
- Keep error strings lowercase and concise, usually without a final period. Include useful context without secrets or personal data.
- Use typed or sentinel errors when callers need to classify a condition; do not compare error strings.
- Log at a clear handling boundary. Avoid logging the same failure at every layer while propagating it.
- Follow [logging guidelines](logging-guidelines.md): use structured `slog` events and never log credentials, cookies, tokens, signed URLs, or raw sensitive payloads.
- Use `defer` for resource cleanup and handle close, rollback, and similar errors where they affect correctness.

## 5. Context, concurrency, and lifecycle

- Put `context.Context` first for I/O, remote calls, and database operations. Propagate the request context from the HTTP boundary.
- With GORM's traditional API, use `db.WithContext(ctx)`. Use context-aware HTTP request constructors for cancellable calls.
- Do not retain request contexts in structs, globals, or background work. Background jobs need application-owned lifecycle, cancellation, and appropriate timeouts.
- Call the cancel function for every context created with cancellation or timeout, typically with `defer cancel()`.
- Every goroutine needs a defined stop condition, cancellation path, and error owner. Avoid unbounded fire-and-forget work.
- Protect shared mutable state with synchronization. Use `go test -race` for concurrency-sensitive changes when supported by the environment.
- Set reasonable timeouts for network calls, external services, queue consumers, and background synchronization. Bound retries, use backoff, and preserve idempotency where needed.
- Support graceful shutdown for servers, workers, and owned resources, following the existing application lifecycle.

## 6. HTTP and API safety

- Treat path, query, header, and body values as untrusted input. Bind into explicit request types and validate before use.
- Use status codes that match the API contract: typically 400 for invalid input, 401 for unauthenticated, 403 for forbidden, 404 for missing resources, 409 for conflicts, and 500 for unexpected internal failures.
- Preserve the response shape specified by the existing OpenAPI contract and router. Do not introduce a universal success or error envelope without an API design change.
- Map internal and business errors at the HTTP boundary. Never expose SQL, stack traces, filesystem paths, credentials, or sensitive upstream responses to clients.
- Collection endpoints need defined pagination and sorting and a reasonable maximum page size; do not load unbounded datasets.
- Update OpenAPI, generated consumers, and relevant tests for public API changes. State a migration approach for incompatible changes.
- Keep middleware behavior consistent with the existing router. Avoid permissive CORS or other broad security settings without a reviewed need.

## 7. Database and GORM

Follow [database guidelines](database-guidelines.md) for this repository's SQLite connection, goose migrations, transaction boundaries, and explicit schema decisions. In particular, use versioned migrations rather than startup `AutoMigrate`.

- Repository queries must propagate context and check GORM errors. Handle `gorm.ErrRecordNotFound` deliberately.
- Never build SQL by concatenating user input; use parameters or safe GORM expressions.
- Updates and deletes need explicit conditions and safeguards appropriate to their scope and authorization.
- Use a transaction when a business operation requires multiple writes to be atomic. Roll back on failure and keep transactions short; do not perform unstable remote I/O inside them.
- Avoid N+1 queries, unbounded reads, and unnecessary full scans. Use pagination, appropriate indexes, preloading, or batching as needed.
- Keep all writes in a transaction on the same transaction handle. Do not treat a request-scoped global `*gorm.DB` as implicit state.
- Use an explicit storage timezone convention (UTC is preferred) and RFC 3339 for API timestamps.

Any schema constraints specific to this product, including the current application-managed relationship and validation policy, are defined in [database guidelines](database-guidelines.md) and take precedence over generic examples.

## 8. Configuration, resources, and security

- Inject configuration through the existing configuration system; validate required values at startup. Never commit plaintext passwords, tokens, or keys.
- Set HTTP client timeouts, connection-pool bounds, and database pool settings deliberately. Close owned resources.
- Validate external URL schemes, hosts, and redirect behavior to prevent SSRF where user-controlled URLs are involved.
- Contain file writes to their intended root. Normalize paths and guard against traversal, absolute-path escapes, and symlink risks where applicable; this matters especially for cloud-drive and STRM paths.
- Validate external API responses and error conditions instead of assuming upstream data is always valid.
- Make batch and synchronization work idempotent, recoverable after failure, and observable through the repository's implemented mechanisms.
- Review third-party dependencies for licensing, maintenance, and security. Keep module resolution recorded in `go.mod` and `go.sum`.

## 9. Testing and maintainability

- Add or update tests for core behavior changes. For bug fixes, add a regression case when practical.
- Use Go's standard `testing` package. Prefer table-driven tests for repeated cases; tests should be independently runnable and order-independent.
- Cover normal behavior, validation, boundaries, important failure paths, and cancellation or timeout behavior when those are part of the contract.
- Use `httptest` for HTTP behavior. Verify real SQLite migration, transaction, and constraint behavior with suitable integration tests.
- Tests must not rely on personal credentials, fixed developer paths, or production services. Provide a repeatable substitute for external dependencies.
- Do not target an arbitrary global coverage percentage. Assert meaningful outcomes on critical paths.
- For concurrency-sensitive changes, run `go test -race ./...` when the toolchain supports it.
- Explain non-obvious algorithm constraints and tradeoffs in code comments.

See [quality guidelines](quality-guidelines.md) for the checks that are actually configured in this repository. The supplied source spec proposes `golangci-lint` and `govulncheck` gates, but those are not currently configured as repository gates; do not claim they ran unless they are installed and invoked. Adopt them only through a separately reviewed tooling change.

## 10. AI-assisted changes

Before changing Go code, read this guide, `backend/go.mod`, the relevant implementation and tests, and the applicable repository-specific guide. Reuse the established directory layout, error behavior, and dependency injection. Do not replace Gin/GORM or add a large framework without an explicit requirement and design review.

Keep each change scoped to the requested behavior. Include necessary input validation, context propagation, error handling, security controls, and tests. When business requirements are missing, follow current API contracts and project design documents rather than inventing incompatible behavior. In the handoff, identify changed files and checks actually run, and state why relevant checks were not run. Never put real secrets or sensitive direct links in tests, logs, configuration, or examples.

## 11. Review checklist

- [ ] Go naming and formatting follow the conventions; no unrelated changes are included.
- [ ] Handler, business, and persistence responsibilities and dependencies are clear.
- [ ] Errors are handled and classified where needed; logs are useful, non-duplicative, and free of sensitive data.
- [ ] Context, timeouts, goroutine lifetimes, and resource cleanup are correct.
- [ ] Input validation, authorization, HTTP status, and response shape match the API contract.
- [ ] Queries, transactions, and migrations follow the repository database policy.
- [ ] Necessary tests and API documentation are updated.
- [ ] Configured repository checks pass; concurrency and security checks are reported accurately when applicable.

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/guide)
- [GORM Context](https://gorm.io/docs/context.html)
- [golangci-lint v2 configuration](https://golangci-lint.run/docs/configuration/file/)
