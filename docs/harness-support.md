# Harness support

Each harness is supported by a plugin: an executable named
`hypercube-harness-<name>` installed beside `hypercube`. The `harness` value in
the [configuration](configuration.md) is that name. `make install` installs
the plugins for Claude Code (`claude`), OpenCode (`opencode`), Codex
(`codex`), and Pi (`pi`). All keep their native UI, conversations, transcripts, credentials, and delegation; the
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
  later), `tool`, `stop`, `failed` (with `error`), `approval`, `answered`, and
  `usage` (with `context`, the tokens of the latest request, `window`, the
  model's context window, and `cost`, in US dollars). Context at `stop`
  continues the turn. Their effects are in
  [report.go](../internal/report/report.go).
- **Waiting messages.** While the agent is idle, the harness runs
  `hypercube wait`. When messages arrive, it prints them and exits with
  status 2; the plugin starts a turn with them. It exits 0 without output
  when a newer waiter replaces it or its parent process ends.

The plugins are in [plugins](../plugins).

## Messages and native subagents

Reports deliver messages at prompts, tool completion, and turn end. Pending
messages can continue a turn. For an idle conversation, a background waiter
uses Claude's `asyncRewake` mechanism, OpenCode's session prompt API,
`codex queue`, or Pi's `sendUserMessage` to start a native turn. None of these
types into the pane, so a user's unsent
draft stays in its editor.

A failed worker turn preserves its queued messages and sends its team's
lead a bounded error summary. The lead's own failures remain
in its pane.

Native subagents are separate from the Hypercube agent's main conversation.
Plugins report only the main conversation, so a subagent never takes the
parent's messages or changes its saved session, activity, or waiter. Claude
and Codex identify subagents through hook input; OpenCode identifies child
sessions through their parent. Pi has no native subagents. A native subagent's Claude approval still needs
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

## Codex

The plugin runs the Codex CLI with Approve for me: an automatic reviewer
answers approval requests inside a workspace-write sandbox with network
access. The agent's Git directories are writable, so a worker can commit in
its worktree. Each launch trusts its own hooks, so no user settings change.
Codex fires no hooks while idle, so the pane runs the plugin, which starts
Codex and adds waiting messages to its session with `codex queue`. Folder trust
remains Codex's own prompt.

## Pi

Pi 1.4 or later is required. The plugin loads an extension with `-e` that
registers the Hypercube MCP server and reports each run. A conversation
resumes by its Pi session ID. Pi has no approvals. The model is
`provider/id`, and effort is Pi's thinking level.

Pi resolves a partial model ID to another model. The extension rejects the
first prompt when the selected model is not the requested one, and reports
the failure.

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

- **Codex failures and interruptions:** Codex has no failure or interruption
  hook. A failed or interrupted turn leaves the agent busy until its next
  prompt, and a failed worker turn does not notify the lead.
- **Codex tool choice:** Codex's native multi-agent tools stay available. In
  live tests with gpt-6-luna, agents occasionally used Codex's own
  `wait_agent` instead of ending the turn to receive Hypercube messages.
- **Lost wake messages:** OpenCode and Pi start an idle turn through an API
  that does not report a rejected prompt, such as during a manual compaction.
  The messages taken for that turn are then lost and the agent stays busy.

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
