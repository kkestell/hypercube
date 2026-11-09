# Obsolete plugin and runtime protocol paths: implementation

- Task ID: `2026-11-08-005-remove-protocol-compatibility`
- Status: implemented; awaiting independent work review.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md`
- Accepted plan blob: `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89`.
- Accepted review (repository-relative):
  `.agents/reviews/plans/2026-11-08-005-remove-protocol-compatibility.md`, Round 2.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`
  (Hypercube-managed worktree, the task checkout).
- Branch: `hypercube/2f4c7990/compat-impl`; clean tree at
  `bc0f339b5e96e1a8508b70be55560e73a80916e3` (004 review Round 2) before edits.
- Fixed Code base: `bc0f339b5e96e1a8508b70be55560e73a80916e3`.
- Code head: `0d708e9a3f04976976cfd5e719270536fae8e3ef`.
- Implementation commits: `0d708e9a3f04976976cfd5e719270536fae8e3ef`
  (`X-Stage: work`). Verified tree `f7eea26203070ea9f7570ef22e9f06928ff1b05d`
  was written before the commit and is the committed tree.
- Author: `compat-impl`, Hypercube child of `2f4c7990`; native conversation
  `1a622741-22c7-4c26-9705-f8832685efee` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed to the session.
- Work review (repository-relative, not yet written):
  `.agents/reviews/work/2026-11-08-005-remove-protocol-compatibility.md`.

## Accepted guidance

These blobs matched the accepted Round 2 inputs before editing.

| Guidance path (repository-relative) | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `f6bf8eca928d621d7eb122d1f19a7bd86053896e` |

## Changes

- `internal/session/grants.go:recordAllow`: the answer command now carries
  `Choices: []string{item.Choices[0]}`. Request ID, first-choice meaning,
  waiter handling and grant scope are unchanged. AlwaysAllow and automatic
  approvals in `event.go` share this producer. `commands.go:send` still
  replays stored command bytes unchanged.
- `internal/harness/protocol.go`: removed `Command.Choice` and
  `StartArgs.Hypercube`.
- `plugins/internal/plugin/serve.go`: removed the scalar-to-array answer
  conversion. A start with an empty `MCP` is rejected after the nil-start
  check and before the native start function runs.
- `plugins/internal/plugin/plugin.go`: removed `MCPCommand`. Claude and Pi
  write `"command": a.MCP, "args": []`; OpenCode uses `[]string{a.MCP}`.
  Scripted `createSessions` checks for an empty `MCP` itself and runs
  `exec.Command(start.MCP)`; scene-only scripted starts need no bridge.
- `internal/harness/harnesstest/harnesstest.go:answer`: computes a local display
  string from `Choices`. The derived log `Entry.Choice` remains a test
  observation.
- `cmd/hypercube/main.go`: removed the `mcp` and `hold` dispatch, their imports,
  usage text and package comment. `agents/architecture.md` drops the sentence
  promising those commands.
- `internal/session/offers.go:answerOffer`: the answer must be one of the saved
  item's choices (`slices.Contains`). The error text is unchanged.
- `internal/session/child_options.go`: a fork with any nonempty directory is
  rejected in `prepareChild`. `forkDir` takes only parent and name: a parent in
  a checkout or non-Git folder is shared, and a parent with `Base` gets a new
  worktree and child branch.
- Tests:
  - Deleted `plugins/internal/plugin/plugin_test.go` (only `TestMCPCommand`)
    and Claude's `TestStartWithLegacyMCPCommand`.
  - Serve's fixture start now names a bridge. Its answer fixture uses
    `Choices`, plus a free-text-only answer. A missing-MCP start case was
    added.
  - `TestAppProcess` runs both `create_session` and `list_sessions` through
    `hypercube-mcp`, and checks that `hypercube mcp` and `hypercube hold` print
    the usage.
  - `TestDismissOffer` lost its saved two-choice variant.
  - `TestForkPlacement` rejects both `.` and another directory, and asserts no
    child was created.
  - `TestAlwaysAllowInSession` asserts each logged answer, both by hand and
    automatic, carries exactly `Choices: ["Allow"]`.

## Deviations

None from the plan's decisions. Notes:

- The unified fork-directory error is "a fork works where its parent works and
  takes no directory". It replaces the two earlier placement-specific
  messages; no test or caller matched their text.
- OpenCode has no existing test of its MCP config `command` array. The plan
  asks existing native tests to keep verifying bridge configuration: Claude's
  and Pi's tests check `"args":[]` with the direct bridge path, and OpenCode's
  one-line change is covered by build and review only.

## Verification

Run in this checkout on macOS/arm64, Go `go1.27.3 darwin/arm64`, Bun `1.4.2`,
against tree `f7eea26203070ea9f7570ef22e9f06928ff1b05d` (Code head).

| Command / inspection | Result |
| --- | --- |
| `go test ./internal/session -run 'TestForkPlacement\|TestDismissOffer\|Grant\|Allow'`, `go test ./internal/session -run AlwaysAllow`, `go test ./plugins/... ./cmd/... ./internal/harness/...` | Pass. Iteration checks. |
| `make check` | Exit 0: browser and website builds, TypeScript, 401 browser tests, 28 desktop tests, `go vet`, gofmt gate, `go test -race ./...` (20 packages ok, including session, plugins and `cmd/hypercube`). Full output: `/tmp/compat-impl-check/make-check-005.out`. |
| `git diff --check`, `git diff --cached --check` | Passed; index held exactly the 20 task paths. |
| `git grep` for `.Choice`, `Choice:`, `.Hypercube`, `MCPCommand`, main `mcp`/`hold` dispatch | Only `harnesstest`'s derived `Entry.Choice` log field remains. The HTTP form field `choice` is untouched. |
| `git hash-object` on plan and guidance | Matched accepted Round 2 blobs above. |

Restart, replay and adoption coverage (`internal/session/restart_test.go`, the
app process test) is unchanged and passed under `make check`. No real sessions
were restarted, nothing was installed, and no user state was touched.

## Session provenance

| Stage / name | Hypercube ID | Native conversation | Harness / model / effort |
| --- | --- | --- | --- |
| Coordinator (this phase) | `2f4c7990` | not supplied | not supplied |
| Earlier coordinator | `a58ee257` | `ses_e41aa9914ffeXSrMrW4OhBDxsQ` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Planner / `compat-plan` | `d36e9339` | `ses_e41a1172bffeqvkaFBnCzmGxvc` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Plan review / `compat-plan-review` | unavailable | `ses_e4195b6fcffe3Mncn7jH9SS1YY` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Implementation / `compat-impl` | child of `2f4c7990`; own ID not exposed | `1a622741-22c7-4c26-9705-f8832685efee` | Claude Code / `claude-opus-5-5` / not exposed |

No blockers. Status stays implemented until independent `xreview work`.
