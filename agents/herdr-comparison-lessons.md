# Herdr comparison and lessons

## What herdr is

Herdr is a terminal multiplexer built for AI coding agents: a single Rust binary
with a persistent server that owns PTYs and a client that attaches/detaches
(locally or over SSH). Layout is `workspace -> tab -> pane`. It classifies each
agent using lifecycle states including `working`, `blocked`, `idle`, `done`,
and `unknown`, supports 15+ harnesses without replacing their native TUIs,
and exposes a CLI plus local socket JSON API that agents themselves can
drive (`pane.split`, `pane.read`, `pane.send_keys`, `pane.wait_for_output`,
`agent.start`, `agent.prompt`, `agent.wait`,
`worktree.*`, `layout.*`, `events.subscribe`).

Verified against Hypercube's source and Herdr's official documentation on
2026-10-25. This comparison does not include live harness smoke tests.
Herdr sources: [overview](https://herdr.dev/),
[socket API](https://herdr.dev/docs/socket-api/),
[agents](https://herdr.dev/docs/agents/),
[agent automation](https://herdr.dev/docs/agent-automation/), and
[CLI reference](https://herdr.dev/docs/cli-reference/).

## What hypercube is

Hypercube is a small team protocol on top of tmux, not a terminal
reimplementation. Go, ~1k LOC, macOS-only, single-user, no daemon. One tmux
session per checkout (`main-vertical`: orchestrator left 40%, workers stacked
right). One JSON state file per workspace holds `agents` and a `messages`
queue, mutated under `flock` with atomic rename.

- Harnesses own UI, transcripts, sessions, permissions, usage, delegation.
  Hypercube supplies identities, worktrees, messages, lifecycle tools.
- Only 3 harnesses (`claude`, `codex`, `opencode`), configured per-launch via
  flags/env/config overrides. `HOME` stays untouched.
- 4 MCP tools: `send_message`, `list_agents`, `create_agent`, `remove_agent`
  (last two orchestrator-only).
- 4 hooks deliver the queue and track activity: `SessionStart` (record session
  ID + inject runtime facts), `UserPromptSubmit` (mark `busy` + deliver),
  `PostToolUse` (deliver mid-turn), `Stop` (block stop if queue non-empty,
  else mark `idle`).
- `send_message` to an `idle` agent nudges via
  `tmux send-keys "Check your Hypercube messages." Enter`.
- Workers each get branch `hypercube/<name>` and worktree
  `<checkout>/.hypercube/<name>` from the orchestrator's `HEAD`. The
  orchestrator merges, then `remove_agent` runs `git worktree remove` and
  `git branch -d` (git refuses dirty/unmerged work).
- No task registry, workflow, or reporting protocol. Agents decide division,
  collaboration, and completion.

## Approach comparison

| Area | Herdr | Hypercube |
| --- | --- | --- |
| Terminal ownership | Own server owns PTYs, survives detach/SSH drop, restores layout | Delegates to tmux (`new-session/split-window/kill-pane/send-keys`) |
| Agent awareness | Screen detection or explicit integration reports; includes blocked and completion states | Explicit `busy/idle` + message queue via hooks/MCP |
| Collaboration | Shared CLI/socket surface for inspecting and controlling agents and panes | Orchestrator manages lifecycle through MCP; peers message; trusted agents also have shell access |
| Git | Generic `worktree.create/open/remove`; reuses existing branch or creates from base; groups workspaces | Fixed `hypercube/<name>` branch + `.hypercube/<name>` worktree; orchestrator merges |
| Harnesses | 15+; manifests describe detection rules, with code/integrations for other behavior | 3 explicit launch builders |
| Human interface | Mouse-native TUI, notifications, layouts, plugin marketplace | Attach to tmux session; inspect state JSON or tmux directly; no dedicated status CLI |
| Scope | General runtime + agent-callable orchestration surface | Minimal identities + worktrees + messages + lifecycle |

Herdr owns the terminal runtime and exposes broad automation. Hypercube
delegates terminal persistence to tmux and supplies a narrower team protocol:
one orchestrator, workers in worktrees, and MCP + hooks for messages. Its MCP
role restrictions are conventions enforced by those tools, not isolation
between trusted local processes.

## Lessons hypercube could adopt

Favor changes that address demonstrated failures while preserving the existing
principles: small implementations, no daemon, and harness-owned UI. The order
below reflects concrete failure paths and implementation cost, not measured
failure frequency.

### 1. Recover partial lifecycle failures

Herdr's `worktree.create` checks out an existing local branch or creates one
from a base. Hypercube's `AddWorktree` always runs
`git worktree add -b <branch> <dir> HEAD`. Branch reuse is useful, but does not
by itself make creation idempotent: the directory or worktree registration may
already exist, or the branch may be checked out elsewhere.

There is a concrete failure path in `createAgent`: worktree creation succeeds,
harness launch fails, and the agent record is removed while the branch and
worktree remain. Retrying creation then fails on those leftovers.

Startup has a separate recovery gap. If the tmux session exists, `start()`
attaches immediately without reconciling agents or panes. Otherwise it
relaunches the saved agents without checking their worktree directories first.
Targets are replaced only after every launch succeeds, so a partial launch
failure leaves newly created panes without updated targets in state.

Prioritize explicit recovery of these partial operations. Reuse resources
only when the repository, branch, directory, and agent identity agree; report
conflicts without resetting branches or deleting work. Reconciliation should
recognize live panes before relaunching agents and preserve queued messages
and saved sessions. This can run during creation or attach without a daemon.

Sources: [Herdr worktrees](https://herdr.dev/docs/cli-reference/#worktrees),
`internal/workspace/workspace.go` (`AddWorktree`), `internal/mcp/mcp.go`
(`createAgent`), and `cmd/hypercube/main.go` (`start`).

### 2. Make inspection convenient when existing commands are insufficient

Herdr provides pane and agent reads through the same API its CLI uses.
Hypercube has no dedicated equivalent, but the orchestrator is not blind:
trusted agents can already read the state JSON, inspect Git, and invoke
`tmux capture-pane` through their shell tools.

Try those existing commands during debugging before adding an interface. If
repeated use warrants a wrapper, distinguish two purposes:

- `status`: stored activity, queued message counts, and current pane existence.
  Stored `busy`/`idle` alone cannot explain a dialog or prove liveness.
- `read`: a bounded capture of the target pane's visible output. This can help
  diagnose a dialog, but is not a complete transcript or structured result.

Either can stay read-only and avoid taking over the harness UI. An
orchestrator-only MCP wrapper would be a convenience, not a security boundary.

Sources: [Herdr socket API](https://herdr.dev/docs/socket-api/),
`internal/store/store.go`, and `internal/mcp/mcp.go` (`list_agents`).

### 3. Harden nudges around demonstrated failures

Herdr still submits prompts through terminal input. `agent.prompt` sends text
and encoded Enter, honors bracketed-paste mode, rejects agents already
detected as blocked, and can order submission and waiting in one request.
It can also prompt an agent that is working. Its advantage is coordinated
submission and readiness checks, not elimination of keystrokes.

Hypercube's `SendKeys` performs two separate tmux calls with a delay:

```go
run("send-keys", "-t", target, "-l", text)
time.Sleep(200 * time.Millisecond)
run("send-keys", "-t", target, "Enter")
```

This appends to half-typed input. Concurrent senders or human input can also
interleave with the text/Enter sequence. A duplicate nudge does not redeliver
messages already removed from the queue, but can still start another turn.
Queue delivery and harmless terminal input are different guarantees.

The repository lists open-dialog behavior under "To verify"; it is not an
established failure across every harness. Smoke-test that case and concurrent
nudges before choosing a fix. Consider serializing or coalescing submissions
if those tests demonstrate interleaving or redundant turns. Neither technique
alone protects a human's partially typed prompt or detects a dialog. Prefer a
native injection API only if the running harness exposes one that preserves
its interactive session and configuration.

Sources: [Herdr prompt delivery](https://herdr.dev/docs/agent-automation/),
`internal/tmux/tmux.go` (`SendKeys`), `internal/mcp/mcp.go` (`sendMessage`),
and `AGENTS.md` ("To verify").

### 4. Defer `blocked` until there is a reliable signal

`blocked` could help distinguish productive work from a worker waiting for
input. But adding an `Activity` value is the easy part. Hypercube's four
registered events do not identify dialog entry or exit, and the hook input
currently contains only a session ID. Reliable reporting would require
additional harness-specific signals and transitions back out of `blocked`.

Herdr uses screen detection or explicit integration reports. Its documentation
warns that unfamiliar dialogs may appear idle. Hypercube also launches all
three harnesses with permission bypass, reducing approval-dialog cases while
leaving startup dialogs and other questions possible.

Add `blocked` only after identifying a recurring problem and a reliable signal
for the relevant harness. Do not infer it from elapsed time or treat an idle
state as proof that terminal input is safe.

Sources: [Herdr blocked detection](https://herdr.dev/docs/agents/#blocked-state),
`internal/hook/hook.go`, and `internal/harness/harness.go`.

### 5. Keep explicit harness builders until abstraction earns its cost

Herdr's manifests describe screen detection rules. Its documentation says a
new detected agent still requires a binary update for process detection,
labels, and integration behavior; manifests alone do not supply full support.

Hypercube's three builders differ in configuration formats, resume syntax,
environment handling, and OpenCode's generated plugin. A table covering all
of this would need callbacks or its own configuration language. Keep the
switch, which already shares hook event names and basic launch setup. Extract
more only when an actual fourth harness reveals useful duplication.

Sources: [Herdr manifests](https://herdr.dev/docs/agents/#detection-manifests)
and `internal/harness/harness.go` (`Command`).

## What not to copy

- Owning a multiplexer, terminal renderer, or persistent Hypercube server
  would expand responsibility beyond the current design and conflict with the
  no-daemon principle.
- Multi-machine SSH, a plugin marketplace, a mouse TUI, and notifications have
  no demonstrated requirement here. They are scope choices; SSH and
  notifications do not inherently violate the existing principles.
- Keep task graphs and approval workflows with the agents and harnesses.
  Hypercube explicitly excludes a task registry, workflow, or reporting
  protocol. Herdr does expose live events and bounded, non-durable event
  history; that is distinct from a durable workflow log.
- Supporting 15+ harnesses is unnecessary for the three currently configured
  here. Add breadth when Kyle needs another harness.

For event retention semantics, see
[Herdr event subscriptions](https://herdr.dev/docs/socket-api/#event-subscriptions).

If only one change is made, address partial lifecycle recovery. Improve
inspection next if direct state/tmux commands prove cumbersome, then harden
nudges around observed failures. Defer `blocked` and a harness table until
their prerequisites and practical benefit are established.
