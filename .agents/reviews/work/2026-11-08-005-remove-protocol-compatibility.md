# Work review: obsolete plugin and runtime protocol paths

## Round 1 — 2026-11-08

**Verdict: accept.** No findings. The grant producer and the scalar `choice`
removal change together, nothing current runs `hypercube mcp` or
`hypercube hold`, and the fork and offer checks match the plan. This is an
independent review by a worker that did not plan or implement the unit.

- Task ID: `2026-11-08-005-remove-protocol-compatibility`; mode: `work`.
- Author: `compat-review`, Hypercube child of `2f4c7990`; native conversation
  `c805986c-9023-4e3e-a660-11912f975ea2` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`,
  branch `hypercube/2f4c7990/compat-impl`.
- Fixed Code base: `bc0f339b5e96e1a8508b70be55560e73a80916e3` (ancestor of
  head, verified).
- Reviewed Code head: `0d708e9a3f04976976cfd5e719270536fae8e3ef`, tree
  `f7eea26203070ea9f7570ef22e9f06928ff1b05d`; the single implementation commit.
- Checkout HEAD at review: `9135c5e8c7a83d26059bbd036e788476797aaaec`; Code
  head to HEAD changes only the work log (`54bef47`, `9135c5e`). Working tree
  clean.
- Work log (repository-relative):
  `.agents/work/2026-11-08-005-remove-protocol-compatibility.md`.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md`, blob
  `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89`, matching the accepted Round 2 plan
  review.

### Assessment

- **Grant answers (plan review Round 1 finding).** `grants.go:recordAllow` now
  records `Choices: []string{item.Choices[0]}` in the same commit that deletes
  `Command.Choice` and the conversion in `serve.go`. Both producers go through
  it: AlwaysAllow (`grants.go:81`) and automatic approval (`event.go:204`).
  `TestAlwaysAllowInSession` checks that every logged answer, by hand and
  automatic, carries exactly `["Allow"]`. Command rows already saved with a
  scalar `choice` are safe. Rows belong to one plugin launch
  (`launch.go:302` deletes them when the launch ends), and `commands.go:send`
  replays them only to that launch's plugin, which is the pre-upgrade build
  that still reads `choice`. A newly built plugin only ever receives rows
  written by the new producer.
- **Removed subcommands.** The backend starts helpers by path from
  `cmd/hypercube/programs.go` (`hypercube-mcp`, `hypercube-hold` beside the
  executable), and `Makefile` and `make/macos.mk` build and sign both. A search
  of Go, scripts, desktop and make files finds no invocation of
  `hypercube mcp` or `hypercube hold`. The only match is `scripts/dev:79`,
  which matches old holder command lines to stop them, not to start them;
  removing that is unit 006's. Harness MCP configuration is written per launch
  from `StartArgs.MCP`, so no installed configuration points at the main
  executable. Holders and plugins already running keep their own binaries.
- **Start validation.** Serve rejects an empty `MCP` after the nil-start check
  and before the native start function; `TestServeBeforeStart` covers it.
  Scripted `createSessions` checks at its own use, so scene-only starts are
  unaffected.
- **OpenCode MCP command.** `[]string{a.MCP}` produces the same array as the
  old `append([]string{command}, args...)` with `args` empty, which was the only
  current case. Reading the code is enough to show the output is unchanged, so
  the missing direct test is not a gap.
- **Fork directory.** The MCP tool already rejects a fork with a directory
  (`internal/mcp/children.go:134`), so only proposals saved by old versions
  could reach the removed acceptance. `prepareChild` now rejects any directory
  for a fork before any lookup or creation, and the single message replaces the
  two placement-specific ones without losing information: both said to drop the
  directory. `TestForkPlacement` checks `.` and another folder fail and that no
  child was created. `forkDir` is now a two-branch placement function.
- **Offers.** `answerOffer` accepts only the saved item's choices; current
  offers always save all three (`offers.go:36`), and `TestDismissOffer` keeps
  the invalid-choice, Dismiss and no-child checks.
- **CLI and docs.** Usage, package comment and `agents/architecture.md` no
  longer mention the commands; `TestAppProcess` runs both MCP tool calls
  through `hypercube-mcp` and checks that `mcp` and `hold` print the usage.

### Verification

| Check | Status | Source |
| --- | --- | --- |
| `make check` on tree `f7eea262…` | reused, exit 0, 20 Go packages ok including `cmd/hypercube`, `internal/session`, `plugins/opencode` | `/tmp/compat-impl-check/make-check-005.out`; log Verification |
| Anchors, ancestry, tree, plan blob, Code head→HEAD diff | run | `git merge-base --is-ancestor`, `git rev-parse`, `git hash-object`, `git diff --stat` |
| Remaining `Choice`, `Hypercube`, `MCPCommand`, old-version comments; callers of `hypercube mcp`/`hold` | run | `git grep` at Code head over Go, scripts, desktop, make files |
| Command replay lifetime | run (inspection) | `commands.go:send`, `launch.go:302`, `restore.go:184` |

No new test run: each open question was settled by reading code, and the
reused `make check` covers the changed packages at Code head.

Limitations: no real installed sessions were restarted, as the plan excludes it.
