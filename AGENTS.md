# Hypercube

Hypercube runs one team of local coding agents per workspace using Go and
tmux. An orchestrator works in the original checkout; workers use Git
worktrees or existing directories. Claude Code and OpenCode own their native
UI, transcripts, sessions, permissions, credentials, and native delegation.
Hypercube supplies identities, profiles, worktrees, messaging, and lifecycle.

Kyle is the only user. Favor small implementations that are easy to understand
and change. There is no task registry, reporting protocol, or workflow engine.
Multiple teams and scheduled tasks are outside the current scope.

## Principles

- tmux owns PTYs, rendering, input, scrollback, selection, resizing, and pane
  lifetimes. Do not rebuild a terminal emulator, proxy, or custom renderer.
- A detached session keeps running. Processes start only for explicit starts,
  worker creation, or the documented relaunch of an exited harness. Stop and
  delete close the session or worker explicitly.
- Configure harnesses only with per-launch arguments, environment, and
  overrides. Leave global and project settings, credentials, and HOME alone.
- Messages go through hooks and native harness wake-ups. Never type a nudge
  into a pane: it can corrupt the user's draft prompt.
- Native subagents must not consume or replace the parent conversation's
  messages, session ID, activity, or waiter. Their approvals still need
  attention in the same pane.
- Never trust a saved pane number as ownership. Check its live workspace and
  agent markers before closing or relaunching it.
- A failed write keeps queued messages; failed lifecycle cleanup remains
  recorded for retry. Refuse dirty or unmerged worker worktrees.
- State writes are locked and atomic; unchanged updates leave timestamps alone.
- macOS only. Agents are trusted local processes with the user's credentials.

## Layout and documentation

- `cmd/hypercube/main.go`: CLI, workspace lifecycle, harness exec, relaunch.
- `internal/store`: JSON state, locks, atomic updates, queues.
- `internal/workspace`: checkout discovery, worktrees, ignored-file copying.
- `internal/harness`: profiles, launch overrides, embedded OpenCode plugin.
- `internal/tmux`: session and pane operations, ownership checks, titles.
- `internal/mcp`: stdio JSON-RPC tools for messages and worker lifecycle.
- `internal/hook`: context injection, steering, stop and failure events.
- `internal/wait`: idle message delivery with persisted waiter generations.
- `internal/statusline`: Claude Code's status line and approval clearing.
- `docs/architecture.md`: processes, state, ownership, lifecycle, messaging.
- `docs/configuration.md`, `docs/harness-support.md`, `docs/installation.md`:
  profiles, native integration, and building/running.
- `docs/teamwork.md`: teamwork practices; historical plans are in `agents/plans`.

Keep architecture and integration documentation current when changing them.
Code and current docs stand alone without referring to predecessor code.

## Build and validation

Use `make` to build `bin/hypercube`. Before reporting behavior changes done,
run `make check`: Go build, vet, formatting check, and race-enabled tests.
Tests force user-visible message, ownership, and lifecycle behavior, using
isolated state and real tmux with local harness stand-ins. Bun runs the
OpenCode plugin test; do not install browser tooling or spend tokens on agents
for routine validation. Test processes and resources must be uniquely owned
and cleaned up. Never stop the user's tmux server or installed workspaces.

For documentation-only changes, use focused searches and diff inspection.
Do not run full suites for comments, docs, or ordinary file renames.

`make install` installs to `~/.local/bin`. The user runs `make reinstall`
outside Hypercube: it stops every installed workspace, installs, and restarts.
Do not run it yourself. `make dev` opens an isolated Ghostty test team;
`make dev-stop` stops it, and `make dev-clean` deletes its disposable resources.
Clean up any development sessions you started before finishing.

## Working and communication

Do less. Use idiomatic Go and clear ownership, plain data, and direct functions.
Avoid abstractions and configuration without a concrete need. Return clear
errors for bad input. Comments explain why; tests check observable behavior.

Do not commit, create branches, change the task title, or offer to commit unless
asked. Present review findings; do not automatically fix, stage, or commit them
without an explicit request. Keep verification and tool use proportional.

Keep required skill notices to one brief sentence. Do not send opening
acknowledgements, announcements of intent, or generic narration. Intermediate
updates contain concrete findings, material risks, blockers, or necessary
questions. Describe actions as complete only after tool results confirm them.
