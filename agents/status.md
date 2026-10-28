# Status

Features of the Rust port (`agents/plans/2026-10-27-002-rust-port.md`) and
their implementation status as of 2026-10-27. Phases 0 to 3 are on `main`,
reviewed, and covered by tests. Phase 4 has not started.

Status values: **Done**, **Not started**, **Dropped**.

## Commands

| Feature | Status |
| --- | --- |
| `hypercube`: resolve the workspace, create the orchestrator on first run (`no_git` outside Git), launch every agent resuming saved sessions, workers with the nudge prompt | Done |
| `hypercube` when the workspace is running: report the terminal that hosts it | Done |
| `hypercube` attaching to a running workspace | Not started (phase 4) |
| `stop [dir]` through the control socket | Done |
| `stop --all` with listing and confirmation | Done |
| `delete [dir]`: refuse dirty or unmerged worktrees, stop, remove worktrees and branches, state, locks, and socket | Done |
| `delete --all`, reporting refusals and continuing | Done |
| `status`: directory, running, agents, created, active | Done |
| `mcp` | Done |
| `hook <event>` | Done |
| `statusline` and `limits` | Dropped: Claude Code's own status line covers it |

## Session, layout, and terminal

| Feature | Status |
| --- | --- |
| One host process per workspace: host lock, control socket, single-threaded poll loop | Done |
| Orchestrator in the left 40%, heavy rule, workers stacked with equal heights | Done |
| Pane labels (name, harness, model, effort, directory), bold when focused | Done |
| Status bar with the checkout path | Done |
| Status bar limits | Dropped |
| Labels and status bar measured in terminal columns (wide and combining characters) | Done |
| No room for another pane: `create_agent` fails; saved agents start hidden until the terminal grows | Done |
| `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` in each pane | Done |
| Kitty keyboard flags and focus events mirrored from the focused pane | Done |
| Ctrl-] cycles focus | Done |
| Click to focus; mouse reports to the pane under the pointer | Done |
| Mouse wheel scrollback, Shift-PageUp scroll mode, "↑N" in the label | Done |
| Drag to select, copy through OSC 52 | Done |
| Input split across reads, lone Esc timeout, bracketed paste passed through | Done |
| OSC 9 and OSC 777 notifications passed to the host terminal | Done |
| Exited agent relaunched in place with its saved session; crash-loop guard leaves it "exited N" after three quick exits | Done |
| Worker whose launch fails: pane shows "failed: …"; the session keeps running | Done |
| Ctrl-\ zooms the focused pane | Done |
| Teardown: SIGHUP, two seconds, SIGKILL, terminal restore, including on panic | Done |
| Detach, reattach, background host, attach from another terminal | Not started (phase 4) |

## State and configuration

| Feature | Status |
| --- | --- |
| `<workspace>.json` under `XDG_STATE_HOME`, locked atomic updates, same format and paths | Done |
| Agent fields, `messages` queue, `no_git` | Done |
| `config.json` with orchestrator spec, named profiles, validation; harness `claude` or `pi` | Done |
| Agents with harness `codex`, `opencode`, or `ox` fail to launch with a clear error | Done |

## MCP tools

| Feature | Status |
| --- | --- |
| `send_message`: queue, refuse self, nudge when idle, report a failed nudge | Done |
| `list_agents` | Done |
| `create_agent`: name and profile validation, worktree or directory, `.worktreeinclude`, submodules, first message, rollback when launch fails | Done |
| `remove_agent`: orchestrator only, merge and cleanliness checks, worktree and branch removal, empty `.hypercube/` removed, pane closed | Done |
| `no_git` variants of schemas and descriptions | Done |

## Hooks, delivery, and harnesses

| Feature | Status |
| --- | --- |
| SessionStart facts for orchestrator, worktree worker, directory worker, `no_git` | Done |
| UserPromptSubmit, PostToolUse, and Stop delivery; response written inside the lock | Done |
| Nudge: text, then Enter 200 ms later with the pane's key encoding; one pending nudge per pane | Done |
| Claude Code launch: MCP config, hook settings, model, effort, resume, prompt | Done |
| Pi launch: extension, model, thinking, approve, session ID, prompt | Done |

## Tests and docs

| Feature | Status |
| --- | --- |
| Unit, round-trip, hold, live, and integration tests (integration uses harness stand-ins) | Done |
| Smoke test with real Claude Code `haiku` and Pi DeepSeek Flash | Done for phases 0 and 1; phase 2 and 3 features not yet smoke tested |
| `docs/architecture.md`, `configuration.md`, `harness-support.md`, `installation.md`, `AGENTS.md` | Done |

## Open items

- Mouse, scrolling, selection, zoom, and relaunch have not been tried in a real
  terminal with real harnesses.
- The host captures the mouse, so the user's terminal selects text only with
  its bypass modifier (usually Shift or Option).
- Pi raises `--thinking medium` to high for models without a medium level; the
  pane label shows the configured effort.
- Claude Code shows messages delivered by the Stop hook as "Stop hook error".
- Claude Code asks to trust a new folder on its first launch there.
- Workers see `create_agent` and `remove_agent`, which refuse them.
