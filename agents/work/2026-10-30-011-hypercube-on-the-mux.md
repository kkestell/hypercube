# Hypercube on the mux

## Plan

`agents/plans/2026-10-30-011-hypercube-on-the-mux.md`

## Summary

Hypercube runs its teams on the mux. `internal/mux` replaces `internal/tmux` and talks to the server through `mux/client`; only the pane ID is saved, as `Target`. The build, `make install`, `scripts/dev`, `scripts/reinstall`, and the integration test use the `mux` executable. Nothing outside `mux/` mentions tmux except the recorded smoke test in `docs/harness-support.md`. The plan's goal is met.

## Departures from the plan

- Labels carry identity only, on the lead's instruction while the work ran: a pane's label is the agent's name, a window's is the team name, a session's is the workspace's base name. No harness, model, directory, or busy/idle text, no colors. The plan's label format and its "busy"/"idle" titles are gone.
- Because the pane label no longer changes, `RefreshActivity` and `Activity` became `RefreshAttention`, which only sets the pane's attention mark. `harness.Launch` labels the pane once, after saving `Target`. The hook, wait, status line, and wrapper paths keep calling it.
- The `docs/usage.md` controls text says a pane's title is the agent's name with the progress its harness reports, instead of `busy`/`idle`.

## Decisions

- `createAgent` finds the team's window through a small `teamWindow` helper, which loads the state and fails with "the session is not running" when `mux.Team` finds none.
- The integration test's pane-typing client attaches once at 200×50 and `select`s each pane before sending input.

## Checks run

- `make check` — passes.
- `go test -count=1 -run TestNativeLifecycle ./cmd/hypercube` — passes with `bin/mux` built by `make build`.

## Manual verification

`make dev` in a Ghostty window; the lead's workers were created and messaged through the lead's MCP tools, called from a shell with the lead's agent ID and the dev state, because nothing here can type into the Ghostty window. The checks read the screen through a throwaway client that attached to the dev session (which closed the Ghostty client) and printed the title and status bar text; the tool lived in `/tmp` and is gone.

1. Start a team: the status bar shows `0 default` on the left and `repo` (the workspace's base name) on the right; the lead's title is `lead`.
2. Create a Claude Code worker (`cc`) and an OpenCode worker (`oc`): both open stacked beside the lead, titled `cc` and `oc`.
3. Message exchange both ways: `send_message` from the lead to each worker; each ran `sleep 5` and replied `pong`, delivered to the lead.
4. Progress icon: a busy Claude Code agent (`cc`, and the lead when it took the replies) shows the mux's wait icon in its title while busy. A busy OpenCode agent (`oc`, busy about 8 seconds running `sleep 5`) never showed a progress icon, so OpenCode does not report progress the mux shows.
5. Approval attention: `hypercube hook PermissionRequest` with `cc`'s environment put the bell on `cc`'s pane, on the `default` team, and on the `repo` workspace; `hypercube statusline` with the same environment cleared all three. This simulates the harness's hook; no real approval prompt was triggered.
6. Remove a worker: `remove_agent` on a temporary worker closed its pane; its worktree was gone.
7. `scripts/dev stop` ended the dev server (`mux ls` reported no server); `scripts/dev up` brought back the lead and both workers with their earlier conversations visible.
8. Detach and reattach: sending Ctrl-b `d` through a client detached it; rerunning `~/projects/hypercube-dev/run` in Ghostty attached again with the same three wrapper processes (same PIDs, no relaunch).
9. `scripts/dev clean` removed `~/projects/hypercube-dev`; `ps` showed no mux server under the dev state and no dev Ghostty window.

An earlier worker created from my shell had no conversation to resume because the shell carried `CLAUDE_CODE_CHILD_SESSION`, which `scripts/dev` strips from its own run script; later workers were created with it unset and resumed.

## Follow-up work

- OpenCode shows no progress icon in its pane title; either it does not emit OSC 9;4 or needs a setting.
- The first `make reinstall` after this change stops the workspaces with the installed tmux-based binary, as the plan says.
