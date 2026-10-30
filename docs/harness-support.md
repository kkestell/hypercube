# Harness support

Claude Code and OpenCode retain their native UI, conversations, transcripts,
credentials, and delegation. Hypercube supplies per-launch configuration
without rewriting user or project settings or changing HOME.

The exact launch arguments and environment are owned by
[harness.go](../internal/harness/harness.go). Shared hook handling lives in
[hook.go](../internal/hook/hook.go); OpenCode's native event adapter lives in
[opencode.js](../internal/harness/opencode.js).

## Messages and native subagents

Hooks inject messages at prompts, tool completion, and turn end. Pending
messages can continue a turn. For an idle conversation, a background waiter
uses Claude's `asyncRewake` mechanism or OpenCode's session prompt API to
start a native turn. Neither path types into the pane, so a user's unsent
draft stays in its editor.

A failed worker turn preserves its queued messages and sends its team's
lead a bounded error summary. The lead's own failures remain
in its pane.

Native subagents are separate from the Hypercube agent's main conversation.
Their hooks must not take the parent's messages or change its saved session,
activity, or waiter. Claude identifies them through hook input; OpenCode
identifies child sessions through their parent. A native subagent's Claude
approval still needs attention in the parent's pane.

## Claude Code

Claude launches in auto permission mode. Tool calls that auto mode does not
allow still open an approval dialog. Hypercube flags these through the
permission hook and clears attention through the native status line when the
dialog is answered, including denials.

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
  another prompt or completed turn.
- **Claude worker resumption:** a resumed worker can remember its
  conversation but refuse Hypercube tool calls. The native `/mcp` menu reports
  all four tools connected while the resumed API request marks them
  unavailable. Asking the worker to use ToolSearch restores the calls.
