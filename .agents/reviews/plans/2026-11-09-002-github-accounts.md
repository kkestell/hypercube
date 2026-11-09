# Plan review: 2026-11-09-002-github-accounts

## Round 1

- Task ID: `2026-11-09-002-github-accounts`
- Mode: `plan`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the planner (`1e20740e-b018-49eb-9ae2-27fe8948db63`).
- Plan (repository-relative): `.agents/plans/2026-11-09-002-github-accounts.md`,
  blob `665bfe3e1eb2079209f553481eb073501290aa8f`
- Guidance blobs: `.agents/designs/2026-11-09-001-github-integration.md`
  `bd05647e353db6d672ce69b41fe047ee89f2f730`
- Source HEAD: `b8025c7d20e951f87fec8a4ca30a721e78ebc39a`

### Verdict

Work size: right-sized. Detail: appropriate. Ready. Action: `accept`.

### Evidence

Independently checked consequential choices:

- **Token path.** `plugins/claude/main.go:71` and `plugins/pi/main.go:112`,
  `plugins/opencode/main.go:95` put `StartArgs.Env` only in the MCP server
  config; each plugin starts its harness with `os.Environ()`
  (`plugins/claude/main.go:98`, `plugins/pi/main.go:134`,
  `plugins/opencode/server.go:37`). `hold.Start` sets the child's `cmd.Env` from
  its `env` argument and writes nothing to disk. Appending to the holder's
  environment in `harnessclient.Launch` (`internal/harnessclient/client.go:28`)
  and `terminal.Set.Open` (`internal/terminal/terminal.go:70`) reaches the
  harness, its tools, and the MCP bridge with no plugin or protocol change. This
  is the simpler of the two mechanisms and the right one.
- **Locking.** `run` (`internal/session/launch.go:123-150`), `Shell` and
  `WorkspaceShell` (`internal/session/shells.go`) call the launch under `r.mu`;
  `Env` reading only the watcher's own map under its own lock avoids I/O under
  the runtime lock, and the watcher never calls into the runtime, so there is no
  lock-order cycle.
- **Account listing.** With gh 2.86.0, `gh auth status --json hosts --hostname
  github.com` exited 0 and listed three logins; `gh help auth status` states the
  command always exits zero with `--json` unless the error is fatal. So an
  invalid or offline token does not make the pass clear every status.
- **`credential.githubAccount` lookup.** `git config --get` returns the
  effective value, including global configuration. That matches what a
  credential helper keyed on the same setting uses for pushes, so `gh` and Git
  agree on the account. On a machine with a global default, push-permission
  detection only runs for repositories whose effective value names no gh login;
  this follows the request ("the repo's `git config credential.githubAccount`").
- **Remote and URL parsing, caching, failure handling** and the stand-in test
  suite (fake gh refusing `GH_TOKEN`, `httptest` GraphQL, isolated Git config)
  are settled and testable as written. `!env NAME` fits the existing keyword
  list in `internal/harness/harnesstest/harnesstest.go`.

No findings.

### Normalization

Active target 600–1,500 words (experimental default; no curated reference set).
Measured 1,497 words, 126 in fenced code (the `Env` hook, `Watcher` interface,
one GraphQL query). Inside the band, near its top. The length carries one
mechanism through three launch sites plus a substantive stand-in test suite,
comparable to the "fix restored file/diff loading plus refresh ownership"
reference with a test-suite share like the "local Git integration suite"
reference. No split or trim warranted.

### Handoff

Proceed as written. Limitation: Remote parsing, watcher, and launch tests were
not run; plan review inspects feasibility only.

## Execution decision

- Author: parent coordinator (Hypercube session 1771c79b, Claude Code, native conversation 2fb9801a-4985-41f3-bd41-371c1ae39e8e, claude-opus-5-5)
- Revision checked: 63165bad85f0b12c1fd122d2c4d70545141755e5
- Final plan blob: `.agents/plans/2026-11-09-002-github-accounts.md` 665bfe3e1eb2079209f553481eb073501290aa8f
- Guidance blobs: `.agents/designs/2026-11-09-001-github-integration.md` 1be166e4fffb44898b53d6c18ad9f4fb89bd4152
- Dispositions: No findings. The plan is unchanged. The only design change is the `Status.Error` comment (token read, fetch, or query), which matches step 3's token read and changes no interface.
- Decision: proceed
