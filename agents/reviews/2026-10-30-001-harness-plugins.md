# Harness plugins review

## Scope and coverage

The staged changes for `agents/plans/2026-10-30-001-harness-plugins.md`: the
launch protocol in `internal/harness`, the `internal/hook` to `internal/report`
rename, `internal/wait`, the CLI in `cmd/hypercube/main.go`, the Claude and
OpenCode plugins in `plugins/`, the integration test's stand-in plugin, the
Makefile, `scripts/dev`, `scripts/reinstall`, and the documentation updates.
Each event's handling, response, and side effects were compared with the
deleted `hook`, `statusline`, and `opencode.js` code.

Gaps:

- No live session with real Claude Code or OpenCode was run. The plugins'
  hook output, `asyncRewake` waits, and the OpenCode JavaScript were checked
  by reading and by the unit and integration tests only.

## Summary

| ID | Severity | Category    | Fix   | Title                                                           |
| -- | -------- | ----------- | ----- | --------------------------------------------------------------- |
| F1 | High     | Correctness | Fixed | Reinstall cannot expand its glob and leaves workspaces stopped  |
| F2 | Low      | Correctness | Fixed | Install copies every `bin/hypercube*` file, including stray ones |

## F1. Reinstall cannot expand its glob and leaves workspaces stopped

High · Correctness · Fixed · `scripts/reinstall:35`

`scripts/reinstall` runs `set -f` before its loop over running workspaces, so
globbing is off when it reaches `install -m 755 "$ROOT"/bin/hypercube*`. The
pattern stays literal, `install` fails with "No such file or directory", and
the script exits after stopping every running workspace without installing
anything or starting them again. Every `make reinstall` hit this.

### Evidence

`sh -c 'set -f; ls "$ROOT"/bin/hypercube*'` in the repository prints
`ls: …/bin/hypercube*: No such file or directory`.

### Fix

The install line runs between `set +f` and `set -f`, so the glob expands and
the loops after it still split only on newlines.

## F2. Install copies every `bin/hypercube*` file, including stray ones

Low · Correctness · Fixed · `Makefile:21`, `scripts/reinstall:36`

`make install` and `scripts/reinstall` installed every file matching
`bin/hypercube*`. Any other binary there is copied into `BINDIR` as well; this
checkout's `bin/` holds a stale `hypercube-agent`, which `make install` would
put on the user's PATH.

### Fix

Both install `bin/hypercube` and `bin/hypercube-harness-*`.

## Checks run

- `sh -c 'set -f; …'` reproduction of F1 — the glob stayed literal.
- `make install BINDIR=<temp dir>` after the fix — installed `hypercube`,
  `hypercube-harness-claude`, and `hypercube-harness-opencode` only.
- The fixed reinstall install line, run under `set -u; set -f` into a temporary
  directory — installed the same three files.
- `sh -n scripts/reinstall` — passed.
- `make check` before and after the fixes — passed.

## Verdict

The core no longer names a harness, and each report, wait, and launch keeps the
behavior of the code it replaces. The one problem in normal use was F1, which
made `make reinstall` stop every workspace and fail; it and F2 are fixed.
