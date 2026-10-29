# Verification log

## Regression evidence

Implementation tested the HEAD adapter in an isolated `/tmp/115-local-client-red` copy using recorder-backed HTTP transport. The repeated-directory/mixed-ID protocol regression failed on the old `/category/get` path and blank metadata; it passes against the local `/files` implementation. Missing-KID validation was demonstrated red then green for all three supported app devices.

Independent trellis-check review found partial optional privilege decoding could incorrectly report active membership. Its regression failed before transactional decoding was added, then passed for both JSON field orders. No other confirmed blockers were found in the full protocol/contract review.

## Final checks (2026-10-30)

- `GOCACHE=/tmp/115-local-client-cache make check`: PASS; Go formatting/vet/backend tests, frontend typecheck/lint/peer dependencies, 8 frontend test files / 37 tests.
- `GOCACHE=/tmp/115-local-client-cache go test -race -count=1 ./internal/adapter/drive115` under backend: PASS.
- `make smoke`: embedded frontend and real Go binary build succeeded. Sandboxed smoke execution could not bind localhost; approved elevated `python3 scripts/smoke.py` then passed health, auth session/login cookie, embedded SPA, reserved-route 404, schema and graceful shutdown checks.
- `go list -m all` validated programmatically: no 115driver/SheltonZhu modules.
- `git diff --check`: PASS.

HTTP protocol tests use an in-memory RoundTripper forwarding requests to httptest.ResponseRecorder because the sandbox denies listening sockets. They exercise method, endpoint, params, headers, credentials, cancellation and parsing without contacting a real account.

## Remaining delivery state

Live 115 account acceptance has not been performed (all three devices, saved credentials, repeated directories and account information). User review/commit approval pending; no code commits, task archive or journal auto-commits have been made.

## User-directed membership follow-up verification

- Cookie `vip_limit?feature=2` selected `data.user_limit.level_name` is propagated verbatim as optional membershipName through the persisted profile, generated API and both UI surfaces; public numeric membershipLevel and numeric legacy normalization removed.
- Membership HTTP regressions cover literal/unfamiliar names, whitespace, missing/null/numeric/malformed/failing responses, identity preservation and numeric-only nav evidence. Service tests cover legacy no-name snapshots and refresh. SSR checks actual escaped visible text for page/sidebar; reviewer found and fixed sidebar name previously present only in aria-label. All were demonstrated failing before fixes.
- After reviewer edits, coordinator reran `make check`: PASS, 9 frontend files / 41 tests plus Go tests/vet/format and frontend type/lint/peer checks.
- Coordinator `go test -race -count=1 ./internal/adapter/drive115`: PASS after membership product edits.
- `make generate` completed; generated Go/TS changes derive from OpenAPI. Module listing reconfirms no 115driver. Final embedded build/smoke results recorded separately below.

Unrecognized concurrent workspace edits: broad indirect dependency version upgrades in go.mod/go.sum and a staged deletion of drive115/.gitkeep. Both implement/check agents and coordinator confirmed they did not perform these edits; they were preserved, not reverted or staged. A future commit must separate SDK-removal changes from unrecognized dependency changes or obtain user direction. No code commits performed.

Final follow-up delivery checks: `GOCACHE=/tmp/115-local-client-cache make build` PASS; approved elevated `python3 scripts/smoke.py` PASS on the rebuilt application (health, auth, embedded SPA, reserved routes, connection schema, shutdown). `git diff --check` PASS. Real 115 account acceptance remains pending.
