# Work review: utility model and automatic session titles

## Round 1

- Task ID: 2026-11-10-001-utility-model-titles
- Mode: work
- Author: implementation reviewer, Hypercube child session `work-reviewer` of
  12efdd39 (Claude Code 2.1.319, Opus 5.5, effort high); native conversation
  `df5e7729-3a7c-4479-8594-3c9686d14d65`. Its own Hypercube session ID is not
  exposed to it. Independent of the planner
  (`9702b77b-3288-4ff3-9270-382b4430ab2e`) and the implementer
  (`458c0415-bad7-44db-ad37-2ed81df27abe`).
- Work log: `.agents/work/2026-11-10-001-utility-model-titles.md`
  (repository-relative), blob `277e4d64b05cccd633b9d4633945475396728749`.
- Plan: `.agents/plans/2026-11-10-001-utility-model-titles.md`, blob
  `07f3b45a2631b762f0f89f79e97f1ea959481cd9`, matching the execution decision
  in `.agents/reviews/plans/2026-11-10-001-utility-model-titles.md`.
- Request: slice 1 of GitHub issue #66.
- Code base: `d14fceecb81e427cc04d3cbf2831151bb7e460f2`.
- Code head: the uncommitted working tree on the base (user ruling: the
  coordinator commits once at the end). Fingerprint of the reviewed tree:
  - `git diff d14fcee -- . ':(exclude).agents/research/utility-model.md' | git hash-object --stdin`:
    `0c2807343b007691b78019236d44ed87e5d956aa` (tracked changes, without the
    staged research note);
  - untracked files: `internal/session/titles.go`
    `8c0ce9031bc286ae9275eca99096938a2a84faf8`,
    `internal/session/titles_test.go` `6434999461f7679f85f5b2bc06b0987797f99085`,
    `plugins/claude/generate.go` `f5c8524bed47debae01e5862f4486ee9f8984d48`,
    `plugins/claude/generate_test.go` `b19eb6643082ccf45f686f11f21489fb547af838`,
    `plugins/internal/plugin/main_test.go`
    `67d8b8bcf7891843b2c41b575945d3778079b29c`.
- Excluded workflow records: the research note, plan, plan review, and work log.

### Verdict

`changes-required`. The change does what the request and plan ask, and I found
no correctness defect. Two small findings in the lifetime code the coordinator
asked about remain: a lock in `generate` that guards nothing shared, and Quit
and Detach cancelling utility calls later than the plan says. Both fixes are a
few lines and need no plan change.

### Findings

1. **`generate` guards single-owner state with a mutex (minor, required).**
   `plugins/claude/generate.go:42-117`. `failure`, `text`, `got`, and `plain`
   are written only by the `Read` handler, and the caller reads them only after
   `<-read`, which happens after the last handler call. On the context path the
   caller returns without reading them. So `mu` protects nothing, and a reader
   has to work out that the closure, `fail`, and the tail all share state
   before finding that they don't. `docs/dev/development.md` asks for one owner
   over shared state. Correction: let the reading goroutine own the variables
   and send one value, for example
   `read <- outcome{text, got, failure, plain, p.Read(...)}`, and decide the
   result from that value; drop `mu` and the locking in `fail`.

2. **Quit and Detach cancel utility calls only after agents have stopped
   (minor, required).** `internal/session/sessions.go:257-261, 274-289`, with
   `titles.go:83-94`. The cancels are collected under `mu` but called in
   `stopUtilityCalls`, after `stopLaunches` (Quit) or after every `l.done`
   (Detach). The plan says to call them right after `closing` is set. As
   written, a pending call's shutdown (SIGTERM, then the Claude plugin's `Stop`,
   up to 3 s) runs after the agents' shutdown instead of alongside it, so Quit
   can take a few seconds longer. Correction: call every cancel while holding
   `mu` where they are collected now (a context cancel does not call back into
   the runtime), and only `r.utilityCalls.Wait()` at the end. That also
   replaces `utilityCancelsLocked` and `stopUtilityCalls`, which pass a slice
   between two helpers, with one locked helper and a `Wait`.

### Answers to the coordinator's questions

- **`generate` process lifetime.** The temporary directory is removed by the
  `defer` on every path after `MkdirTemp`. Stdin is closed by `fail`, after a
  `result`, and by `Stop`. On cancellation, `Stop` closes stdin, waits up to
  3 s, then kills; the reading goroutine ends when the plugin exits. This is
  the same shape as `models` (`plugins/claude/discovery.go:80`).
- **`--effort` before `check`.** Correct as written. `serve` does the same
  (`plugins/claude/main.go:80,116`): the effort goes on the command line and
  `check` runs on the `initialize` reply before any prompt. Checking first
  would need a second `claude` launch. Claude's warning is written to stderr
  before the plugin's own error line, and `harnessclient.run` reports the last
  stderr line, so the logged reason is the plugin's (the live run confirms the
  order).
- **Session lifecycle.** `nameSession` checks `closing` and calls
  `utilityCalls.Add` under `mu`, and Quit and Detach set `closing` under `mu`
  before waiting, so no `Add` can follow `Wait`. Each goroutine removes its own
  cancel under `mu` and calls `Done` once. `nameSession` is called without
  `mu` held in both `create` and `createChild`.
- **`Renamed`.** Set only by `Runtime.Rename`, whose only caller is the
  browser's rename action (`internal/ui/actions.go:140`). No other code writes
  `Title` besides `applyTitle`. No `set_title` reference remains outside the
  kept transcript title entry and the test that the instructions omit it.
- **Store migration.** The column is in the schema, `addColumn`, `AddSession`,
  `SaveSession`, `sessionColumns`, and `scanSession`; columns are named, so the
  different position in migrated and fresh tables does not matter. The reopen
  test covers an old row reading `false` and the flag saving.
- **Settings.** Matches the plan's corrected layout: the group heading is the
  only visible heading, with `ModelSelect` (accessible name "Utility model"),
  a ghost **Clear**, and the help text, copied from the Review model row.
- **Docs.** Accurate against the code, written as what the product does, with
  the Rename fact owned by `configuration.md` and linked from `usage.md`.
- **Tests.** Each test protects a decision in the plan. The Quit test is
  meaningful: without cancellation the held call would run for the
  one-minute `utilityTimeout`, past the test's 20 s limit. The `!utility-hold`
  stand-in uses `select {}` like the existing models hold; the test binary's
  timeout timer keeps Go's deadlock detector from ending it early.

### Verification

- Reused: `make check` from `/tmp/check.log` (finished 05:38:33): Go build,
  `tsc`, UI `bun test` 465 pass, desktop `bun test` 28 pass, `go vet`, gofmt,
  `go test -race ./...` all `ok`. Every changed Go file predates the log by
  more than two minutes, consistent with the race run (`internal/session`
  alone took 100 s) covering them.
- Reused: the live `generate` runs (haiku/low success; `minimal` refused) and
  the `make dev` rename, as recorded in the work log.
- Run: `bun test web/app/settings-dialog.test.tsx` (11 pass) and
  `bunx tsc -p .` in `internal/ui` (no errors) on the reviewed tree.
  `settings-dialog.test.tsx` was modified at 05:36:42, close enough to the
  check's start that the log alone did not show it was included.
- Not run: no new Go tests or live model calls; the findings are about
  structure and ordering, not observed failures.

### Limitations

- The reviewed code is uncommitted. A later round compares the fingerprint
  above.
- Linux was not exercised.

### Execution decision

- Author: coordinator, Hypercube session 12efdd39 (Claude Code, Opus 5.5);
  native conversation `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3`.
- Finding 1: resolved. `plugins/claude/generate.go`: the reading goroutine
  owns its `outcome` and sends it with `Read`'s result; `mu` is gone.
- Finding 2: resolved. `cancelUtilityCallsLocked` runs under `mu` right after
  `closing` is set in `Quit` and `Detach`; `utilityCalls.Wait()` runs at the end.
- The coordinator read both fixes against the findings. The user asked for no
  further review rounds, so the repair has no second independent round.
- Evidence (reused from the implementer, after the repair):
  `go test -race ./plugins/claude ./internal/session` ok; `make check` exit 0
  (`/tmp/check2.log`, no failures, newer than every repaired file).
- Decision: accept.
