# Retired build and installation cleanup: implementation

- Task ID: `2026-11-08-006-remove-install-compatibility`
- Status: implemented; awaiting independent work review.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-006-remove-install-compatibility.md`
- Accepted plan blob: `f6bf8eca928d621d7eb122d1f19a7bd86053896e`.
- Accepted review (repository-relative):
  `.agents/reviews/plans/2026-11-08-006-remove-install-compatibility.md`, Round 2.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`
  (Hypercube-managed worktree, the task checkout).
- Branch: `hypercube/2f4c7990/compat-impl`; clean tree at
  `ae546fee7dbb8a09a6edf4770011e23813193bb5` (005 work review) before edits.
- Fixed Code base: `ae546fee7dbb8a09a6edf4770011e23813193bb5` (commit).
- Code head: `672bfaaae8158bbd40d7a66325a96442abd54295` (commit).
- Implementation commits: `fa4111e98ba559716f63a1bad98899d91590f1f6`
  (`X-Stage: work`), whose tree is `fdedee42580802231bd2415bd669c8f9bf215311`,
  and `672bfaaae8158bbd40d7a66325a96442abd54295` (`X-Stage: fix`, see Repair 1),
  whose tree is `1c8bd54b22dc202e5eaaad997c66968378c8497e`.
  `git cat-file -t` confirmed both types. The working tree was not edited
  between the checks below and the commit.
- Author: `compat-impl`, Hypercube child of `2f4c7990`; native conversation
  `1a622741-22c7-4c26-9705-f8832685efee` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed to the session.
- Work review (repository-relative, not yet written):
  `.agents/reviews/work/2026-11-08-006-remove-install-compatibility.md`.

## Accepted guidance

These blobs matched the accepted Round 2 inputs before editing.

| Guidance path (repository-relative) | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89` |

## Changes

- `Makefile:binaries`, `make/linux.mk:build`, `make/macos.mk:build`: removed the
  `hypercube-harness-codex` deletions. `make/macos.mk:place` lost the conditional
  removal of that link. The current `PLUGINS` build and link loops are unchanged.
- `make/macos.mk:quit-desktop`: the guard is now
  `[ -n "$(lsappinfo find bundleid=$(APP_ID))" ]`, a current-only running check,
  instead of matching the executable path to tell an earlier combined backend
  from the desktop app. The quit action, `osascript -e 'tell application id
  "$(APP_ID)" to quit'`, is unchanged. The comment drops the earlier-backend
  explanation.
- `scripts/dev:stop_holders`: matches only `*"hypercube-hold "*`. Its PID-file
  scope and exit wait are unchanged.
- `scripts/linux-install`: unchanged. The generic loop that prunes owned links
  that no longer resolve stays.
- `scripts/linux/test-install`: before the zsh reinstall, the test copies the
  installed `hypercube-harness-pi` to `$app/hypercube-harness-extra`, links it
  from `~/.local/bin` and checks that the link resolves. After the stock
  reinstall it asserts the link was pruned. All shell, holder, restart and
  uninstall assertions are unchanged.
- `scripts/linux/test-bundle`: removed the absent-Codex file checks. The
  per-plugin discovery greps and the negative `codex` grep became one exact
  comparison of discovered harness IDs with the bundle's `plugins` list.
- `scripts/linux/test-release`: removed the absent-Codex file checks.

## Deviations

- `quit-desktop` guard: the plan prescribed AppleScript `application id APP_ID
  is running`. On this Mac, `osascript -e 'application id
  "com.example.doesnotexist.compatcheck" is running'` failed to compile with
  error -1728 (exit 1). An unregistered app ID cannot be resolved, so a first
  `make install-service`, which runs `quit-desktop` before `place`, would
  fail. `lsappinfo find bundleid=…` prints nothing for that fake ID and an ASN
  for a running app (`com.apple.finder`). It keeps the plan's decision: a
  current-only running guard, no launch of a closed app, and the same quit.
  The installed app was never queried or quit; both probes used a fake ID or
  Finder.

## Verification

Host: macOS/arm64, Go `go1.27.3 darwin/arm64`, Bun `1.4.2`. All checks ran
against the tree committed as Code head.

| Command / inspection | Result |
| --- | --- |
| `make check` | Exit 0: browser and website builds, TypeScript, 401 browser tests, 28 desktop tests, `go vet`, gofmt gate, `go test -race ./...` (20 packages ok). Full output: `/tmp/compat-impl-check/make-check-006.out`. |
| `scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install'` | Exit 0 in a disposable Debian container (Docker `linux/aarch64`): `test-bundle: passed`, `test-install: passed`. This covers the extra-plugin link pruning, zsh, fish and bash service starts, holder survival across restart and reinstall, and uninstall. Full output: `/tmp/compat-impl-check/linux-006.out`. |
| `bash -n scripts/dev scripts/linux/test-bundle scripts/linux/test-install scripts/linux/test-release` | Passed (all declare `#!/usr/bin/env bash`). |
| `make -n quit-desktop` | Shows the `lsappinfo find bundleid=com.github.kkestell.hypercube` guard around the unchanged `osascript` quit. Dry run only; not executed. |
| `git diff --check`, `git diff --cached --check` | Passed; index held exactly the 7 task paths. |

Not run, per the plan: `test-desktop`, `test-release`, host packaging or
install, `make dev-clean`, and any app quit or service restart on this Mac.

## Final compatibility audit

Searched executable Go, TypeScript and JavaScript sources, `Makefile`, `make/`
and `scripts/` with `git grep -i -E "legacy|migrat|upgrade|user_version|earlier
(version|build|install)|older (version|build)|saved by (older|earlier)|hypercube\.(wrap|fixedWidth)|\"hypercube
hold|hypercube mcp\b|exec_command|apply_patch"`, plus `git grep -i codex`.
Remaining matches, classified against the shared design:

- WebSocket `upgrade` in `desktop/connection.test.js`,
  `internal/desktop/desktop.go` and `internal/ui/terminal_test.go`: HTTP
  protocol upgrade, not a version path. Kept.
- `make/macos.mk:74` "replacing the executables of earlier installs": normal
  in-place replacement. Kept.
- `internal/ui/config_test.go:37` case named "legacy profiles": asserts that
  `POST /config` rejects a body with an unknown `profiles` field. This is
  current strict input validation, not a converter, and it overlaps the
  "unknown setting" case. Removed in Repair 1.
- `codex` in Go and TypeScript tests (`internal/config`, `internal/store`
  grants and limits, `web/agent/limits.test.tsx`): arbitrary harness ID strings
  in test data. Kept.
- `docs/storage.md:34–35`: user documentation stating that Codex sessions and
  history are preserved. A current statement. Kept.
- No remaining `Command.Choice`, `StartArgs.Hypercube`, `MCPCommand`, main
  `mcp`/`hold` dispatch, old browser keys, retired tool aliases, store
  migration or config converters, or named Codex build/install cleanup.

## Session provenance

| Stage / name | Hypercube ID | Native conversation | Harness / model / effort |
| --- | --- | --- | --- |
| Coordinator (this phase) | `2f4c7990` | not supplied | not supplied |
| Earlier coordinator | `a58ee257` | `ses_e41aa9914ffeXSrMrW4OhBDxsQ` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Planner / `compat-plan` | `d36e9339` | `ses_e41a1172bffeqvkaFBnCzmGxvc` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Plan review / `compat-plan-review` | unavailable | `ses_e4195b6fcffe3Mncn7jH9SS1YY` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Implementation / `compat-impl` | child of `2f4c7990`; own ID not exposed | `1a622741-22c7-4c26-9705-f8832685efee` | Claude Code / `claude-opus-5-5` / not exposed |

## Repair 1: task-level review findings

Addresses findings F1 and F2 of
`.agents/reviews/work/2026-11-08-006-remove-install-compatibility.md`
(review commit `8b3cf4e932896660d2961766c1d49ba2c02e76d6`), accepted by
coordinator `2f4c7990` and routed to this unit. Code base stays
`ae546fee7dbb8a09a6edf4770011e23813193bb5`. Fix commit and new Code head:
`672bfaaae8158bbd40d7a66325a96442abd54295` (commit; tree
`1c8bd54b22dc202e5eaaad997c66968378c8497e`).

- F1: `internal/ui/config_test.go` deleted the "legacy profiles" case of
  `TestSaveConfig`. The "unknown setting" case still covers rejection of an
  unknown field.
- F2: `agents/development.md` ("Backwards compatibility") and
  `docs/upgrades.md` ("Before upgrading") now state only current behavior,
  using the review's suggested wording. Startup creates missing tables and
  indexes, saved-state changes include a migration, and upgrades preserve
  agents and shells. Release notes cover saved-state changes, and older
  versions do not convert state back. `agents/documentation.md` was read first.
  No link or anchor changed (`git grep backwards-compatibility` finds no links).

Verification on the fix commit's tree:

| Command / inspection | Result |
| --- | --- |
| `go test ./internal/ui -run TestSaveConfig -count=1 -v` | Pass: `TestSaveConfig` (broken JSON, incomplete default, unknown setting, second document) and `TestSaveConfigFailure`. |
| `gofmt -l internal/ui` | No output. |
| Documentation diff inspection | Prose only; no links changed. Per `agents/documentation.md`, no code checks for prose. |
| `git diff --cached --check` | Passed; index held exactly the 3 changed files. |

`make check` was not rerun. The fix removes one test-table entry, which the
focused package test covers, and edits prose. The earlier `make check` and
Linux results on `fa4111e…` cover the rest of the tree.

No blockers. Status stays implemented until independent `xreview work`.
