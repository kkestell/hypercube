# Harness support

Each harness is supported by a plugin: an executable named
`hypercube-harness-<name>` installed beside `hypercube`. The `harness` value in
the [configuration](configuration.md) is that name. `make install` installs
the plugins for Claude Code (`claude`) and OpenCode (`opencode`). Both keep
their native UI, conversations, transcripts, credentials, and delegation; the
plugins configure each launch without rewriting user or project settings or
changing HOME.

## Plugin protocol

A plugin talks to Hypercube in three ways, all as JSON:

- **Launch.** Hypercube runs `<plugin> launch` in the agent's pane with the
  hypercube executable path, an environment for Hypercube processes the
  harness starts, the agent's ID, role, model, effort, saved conversation,
  initial prompt, and state directory on stdin. The plugin prints
  `{"argv": [...], "env": {...}}` and Hypercube runs that command. The input
  type is `LaunchInput` in [harness.go](../internal/harness/harness.go). The
  harness should load `hypercube mcp` as an MCP server with that environment.
- **Reports.** The harness runs `hypercube report <event>` with a JSON object
  on stdin and reads `{"context": "…"}`: text for the model to read now. The
  events are `start`, `prompt` (with `session`, the conversation ID to resume
  later), `tool`, `stop`, `failed` (with `error`), `approval`, and `answered`.
  Context at `stop` continues the turn. Their effects are in
  [report.go](../internal/report/report.go).
- **Waiting messages.** While the agent is idle, the harness runs
  `hypercube wait`. When messages arrive, it prints them and exits with
  status 2; the plugin starts a turn with them. It exits 0 without output
  when a newer waiter replaces it or its parent process ends.

Plugins for Claude Code and OpenCode are in [plugins](../plugins).

## Messages and native subagents

Reports deliver messages at prompts, tool completion, and turn end. Pending
messages can continue a turn. For an idle conversation, a background waiter
uses Claude's `asyncRewake` mechanism or OpenCode's session prompt API to
start a native turn. Neither path types into the pane, so a user's unsent
draft stays in its editor.

A failed worker turn preserves its queued messages and sends its team's
lead a bounded error summary. The lead's own failures remain
in its pane.

Native subagents are separate from the Hypercube agent's main conversation.
Plugins report only the main conversation, so a subagent never takes the
parent's messages or changes its saved session, activity, or waiter. Claude
identifies subagents through hook input; OpenCode identifies child sessions
through their parent. A native subagent's Claude approval still needs
attention in the parent's pane.

## Claude Code

The plugin runs the Claude Code CLI, which works on a Claude Team login.
Claude launches in auto permission mode. Tool calls that auto mode does not
allow still open an approval dialog. The plugin reports these from the
permission hook and reports them answered from the native status line when
the dialog closes, including denials.

The status line shows model, thinking effort, and context usage; leads
also get usage-limit bars. Folder trust remains Claude's own decision.

## OpenCode

OpenCode 2.x is required. Each agent uses a standalone server so it receives
its launch configuration. Auto mode approves every permission request in the
TUI. Session tabs are disabled to keep the pane on its assigned conversation.

OpenCode may fall back to a recent model when a requested ID is unavailable.
The plugin rejects the first prompt it observes on a different model, making
a configuration mistake visible instead of silently using that fallback.

## Known limitations

- **Claude activity after interruption:** the integration has no reliable
  interrupted-turn signal. An interrupted turn can remain marked busy until
  another prompt or completed turn. The investigation used Claude Code
  2.1.312; idle notifications and status-line input did not provide a reliable
  substitute.
- **Claude worker resumption:** the live test below found that a resumed
  worker can remember its conversation but refuse Hypercube tool calls. The
  native `/mcp` menu reports all four tools connected while the resumed API
  request marks them unavailable. An explicit request to use ToolSearch
  restored calls in the test. Startup discovery guidance, per-server
  `alwaysLoad`, and `MCP_CONNECTION_NONBLOCKING=0` did not fix the first queued
  turn; those experiments are absent from the implementation.

## Live verification

The latest recorded smoke test, on 2026-10-30, used Claude Code 2.1.314 with
Haiku 5.5 at low effort and OpenCode 2.1.4 with OpenRouter DeepSeek Flash
Latest, in a disposable repository with isolated configuration, state, and
tmux server.

It exercised worker creation in worktrees and existing directories,
bidirectional messages, idle delivery with drafts present, steering during
tools, native-subagent isolation, approval attention, worktree cleanup
refusals, and OpenCode failure reporting and conversation resumption.
The Claude worker resumption failure above prevents a clean pass.
The multi-window implementation has automated coverage with stand-ins but no
recorded live-model smoke test. Automated tests do not establish native
harness compatibility.
