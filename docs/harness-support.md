# Harness support

Claude Code and OpenCode retain their native UI, conversations, transcripts,
credentials, and delegation.

## Messages and native subagents

Messages arrive at prompts, tool completion, and turn end. Pending messages
can continue a turn or start a turn in an idle conversation without typing
into the pane, so a user's unsent draft stays in its editor.

A failed worker turn preserves its queued messages and sends its team's
lead an error summary. The lead's own failures remain in its pane.

Native subagents are separate from the Hypercube agent's main conversation
and do not receive its messages. A native subagent's Claude approval needs
attention in the parent's pane.

## Claude Code

Claude launches in auto permission mode. Tool calls that auto mode does not
allow open an approval dialog.

The status line shows model, thinking effort, and context usage; leads
also get usage-limit bars. Folder trust remains Claude's own decision.

## OpenCode

OpenCode 2.x is required. Auto mode approves every permission request in the
TUI. Session tabs are disabled to keep the pane on its assigned conversation.

Hypercube rejects the first prompt if OpenCode selects a different model than
the one configured.

## Known limitations

- **Claude activity after interruption:** Hypercube has no reliable signal
  for an interrupted turn, so the agent can stay busy in Hypercube's state,
  and idle message delivery waits until another prompt or completed turn.
- **Claude worker resumption:** a resumed worker can remember its
  conversation but refuse Hypercube tool calls. The native `/mcp` menu reports
  all four tools connected while the resumed API request marks them
  unavailable. Asking the worker to use ToolSearch restores the calls.
