# Relaunch in the wrapper

## Plan

`agents/plans/2026-10-30-009-relaunch-in-wrapper.md`

## Summary

`hypercube agent` now runs the harness as a child, relaunches it with the saved conversation when it exits, and after the third quick exit in a row marks the agent exited and exits with the harness's status. `hypercube exited`, `tmux.Restart`, `tmux.Dead`, the `pane-died` hook, and `Agent.Launched` and `Agent.QuickExits` are gone. The plan's goal is met.

## Decisions

- `markExited` stays as a small function (used three times); the launch bookkeeping is inline in the loop.
- `markExited` also clears `Attention`, so the failure paths and the final exit share one write.
- The new integration test removes its agent at the end so the later checks keep their agent indexes.

## Checks run

- `go test ./cmd/hypercube -run TestTmuxNativeLifecycle` — passes, including the new Ctrl-C and three-quick-exits checks.
- `make check` — passes.

## Manual verification

With `scripts/dev up`, driving panes through `tmux send-keys` against the dev server (`env -u TMUX TMUX_TMPDIR=$HOME/projects/hypercube-dev tmux ...`). Run for the Claude Code lead and for an OpenCode worker created through the second dev lead with the `opencode` profile.

1. Ctrl-C reaches the harness only: a draft typed at the prompt is cleared, and the pane stays alive under the same wrapper.
2. Told the harness a word, quit it with `/exit`: it relaunched in the same pane under the same wrapper process and answered with the word (`pelican` for Claude Code, `walrus` for OpenCode).
3. Quit it three times, each within 10 seconds of its launch: the pane died with status 0, its border read `exited 0`, and the agent's activity was `exited`.

`scripts/dev clean` removed the dev state and closed the window.

## Follow-up work

None.
