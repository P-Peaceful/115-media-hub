# Local 115 Client Implementation Plan

**Goal:** Replace the pinned SDK with a project-owned Cookie client.
**Architecture:** Existing Adapter/BrowserAdapter facade over net/http protocol helpers; refer to 115-station source snapshots and preserve service/HTTP contracts.
**Tech Stack:** Go, net/http, encoding/json, httptest.
**Spec:** design.md and prd.md in this task directory.

## Global Constraints
Only the user-directed membership field/API and display changes; no unrelated API/schema/UI changes; no credential leaks; preserve existing work; no commits without an approved commit plan; ID parsing never uses float64.

## Review Focus
Mixed/large IDs, silent root fallback, malformed upstream success, QR state and device mapping, concurrent credentials and context cancellation.

## Execution
- [x] Read reference 115.go, panpath.go and files115.go plus existing SDK solely as protocol evidence for fields not covered by reference.
- [x] Add failing HTTP-boundary tests for mixed ID path responses, repeated calls, silent root fallback and business errors; run targeted tests to document failure.
- [x] Implement client.go/types.go/errors.go with per-request headers and bounded, cancellable calls; test malformed JSON, HTTP failures and no raw response/URL leaks.
- [x] Implement browse.go against /files with strict identity guards and existing listing/filter semantics; test mixed entries, pagination/search/type parameters and canceled/concurrent requests.
- [x] Implement auth.go/profile.go and adapt Challenge/Driver while keeping facade interfaces; test all QR statuses/devices, Cookie result, identity/root and optional enrichment behavior.
- [x] Replace SDK-based adapter tests with local protocol tests, remove module dependency via go mod tidy, verify go list -m all.
- [x] Run CGO_ENABLED=1 go test ./... and go vet ./... under backend, then make check; run adapter go test -race.
- [x] Independent trellis-check agent reviews/fixes task drift and missing tests; coordinator syncs backend spec and records reference provenance and verification.

Each implementation step uses failing-test then passing-test evidence. No production 115 credentials are needed for automated checks. Real upstream acceptance is explicitly pending.

## User-directed follow-up

- [x] Add vip_limit endpoint test with literal upstream label; failed/missing/malformed label preserves identity and renders unknown.
- [x] Replace public membershipLevel with optional membershipName, remove numeric legacy normalization; update service/HTTP fixtures and generate Go/TS schemas.
- [x] Update connection page/sidebar to show upstream name or unknown, add regression coverage including old snapshots and textual escaping.
- [x] Re-run make check and adapter race, independent check, refresh specs and final verification log.
