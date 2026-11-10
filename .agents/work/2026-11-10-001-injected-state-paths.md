# Go race-test speed

- Task: `2026-11-10-001-injected-state-paths`
- Status: implemented; awaiting the coordinator's independent review.
- Plan: `.agents/plans/2026-11-10-001-injected-state-paths.md`
- Accepted plan blob: `8a99ed08b3a55ab433af127e3e750044322738c3`;
  constructor design approved in coordinator chat, no separate plan review.
  The plan was committed before the user asked to skip further process.
- Code base: `55fdf51766a938dac3160c152e2c94418d234f6a` (original task source).
- Code head: `c9d024eeeccd2819b25909af7caccfc0dffacd34`.
- Implementation commits: `848727d65518d2c2be4dbdb5708f3b3186dd252b`
  (initial test-only pass), `c9d024eeeccd2819b25909af7caccfc0dffacd34`
  (path injection and parallel execution). Review the complete range.
- Author: Hypercube `go-test-speed`, OpenCode, `openai/gpt-6.1-sol`;
  coordinator `b9d8c3e5`. Native conversation IDs and author effort are
  unavailable through this session's metadata tools.

## Changes

`config.File` owns a preferences path and provides `Load`/`Save`.
`store.Open(dir, preferences)` retains `DB.Dir` and passes the preferences file
to its existing migration. `session.New` and `ui.Handler` receive the file;
runtime holders, restoration, terminals, and plugin state use `DB.Dir`.
Startup resolves the existing XDG defaults once and passes them through HTTP
and alert wiring. Database layout, permissions, migration order, configuration
defaults, and application lifecycle are unchanged.

Fixtures use their own paths and `Runtime.Env`. Session tests and independent
GitHub, MCP, UI Go, and workspace tests run in parallel. Repository-local hooks
replace global Git wrappers; hooks are removed after their barrier fires.
Validation-only tests seed saved sessions instead of starting agents/worktrees.
Holder tests invoke real `hold.Main` in their instrumented test binary, avoiding
five nested Go builds and the race-build selection files. No JavaScript changes.

Cuts remove duplicate scope/settings/private-bookkeeping assertions, artificial
closed-database/schema tests, exact input timing detail, redundant removal
cross-products, and synthetic Git failure/retry combinations already represented
by public refusal/admission/recovery cases. Consequential worktree data-loss,
process cleanup, restart, delivery, and permission boundaries remain.

## Measurements and verification

macOS arm64, Go 1.27.3. Baseline full uncached race run: **102.97s**.
Final unchanged uncached runs: **32.99s, 32.32s, 32.87s** (about 68% faster).
These checks apply to Code head above; commits did not change verified code.

| Package | Baseline | Final uncached run |
| --- | ---: | ---: |
| session | 99.421s | 29.997s |
| github | 16.208s | 9.960s |
| ui | 13.829s | 17.382s |
| workspace | 14.820s | 14.074s |
| mcp | 10.981s | 8.797s |

Package times overlap; greater parallel load makes some individual packages
slower even though total wall time drops substantially.

- `go test -race -count=1 -json ./...`: passed on final code, 32.99s.
- Two successive `go test -race -count=1 ./...`: passed, 32.32s and 32.87s.
- `go vet ./...` and `git diff --check`: passed.
- `make check`: passed, 39.84s, including Go build/vet/gofmt/race checks,
  458 browser tests and 28 desktop tests.
- Intermediate compile and fixture-log-read errors were fixed. Holders left
  by the failed UI test process were identified by that run's unique executable
  and stopped; unrelated existing processes were left alone.

Evidence is in this worker's transcript and
`/private/var/folders/x3/s0dy9w311sncwf87160crpw80000gn/T/opencode/`:
`go-test-speed-baseline.json`, `go-test-speed-holder-reuse.json`,
`go-test-speed-unchanged-{1,2}.txt`, `go-test-speed-unchanged-summary.json`,
and `go-test-speed-final-check{,-time}.txt`.

## Remaining limit

The below-30s aim was not reached; the user directed stopping at roughly 33s.
The session package alone profiles at 23s, with 50.18 CPU-seconds: transaction
work accounts for 25.35% of samples, runtime/system work 46.73%, and race trace
switching alone 18.37%. Changing test concurrency to 4 or 20 was slower and was
not adopted. A smaller stand-in executable barely changed startup cost and was
discarded. No database-engine change, process-level test partitioning, or
application lifecycle redesign was attempted.
