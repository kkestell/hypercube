# Status

Hypercube uses Go and tmux, with one team per workspace. Claude Code and
OpenCode run their native interfaces in panes. The orchestrator can create
workers with named profiles, their own worktrees or existing directories,
initial briefs, and queued messages.

The harness integration includes saved-session resume, prompt and tool-hook
steering, stop-hook continuation, idle wake-ups that preserve editor drafts,
worker failure reporting, native-subagent isolation, and approval attention.
Pane relaunches retain saved sessions and stop after repeated quick exits.

Multiple teams, scheduled tasks, and custom terminal rendering are outside
this implementation. Startup refuses existing state with extra teams or
scheduled tasks rather than dropping it.

`make check` is the validation entry point. Harness tests use local stand-ins;
they do not spend model tokens. Real harness UI behavior requires a smoke test.

## Live smoke test, 2026-10-30

Used Claude Code 2.1.314 with Haiku 5.5 at low effort and OpenCode 2.1.4 with
OpenRouter DeepSeek Flash Latest. All work ran in a disposable Git repository
with isolated Hypercube configuration, state, and tmux server.

Verified with actual harnesses and model calls:

- The orchestrator creates Claude and OpenCode workers through MCP, using
  named profiles, a worker worktree, and an existing directory.
- Initial briefs and return messages reach the intended agents.
- Idle wake-ups preserve unsent editor drafts in both harnesses.
- Steering queued during a running shell tool appears in that tool's result
  and changes the worker's next message, in both harnesses.
- A native Haiku subagent completes with the parent's Hypercube session ID
  unchanged. The parent-only message is absent from the child's transcript.
- A Claude Bash approval sets the attention flag and red pane indicator;
  approving the harmless command clears the flag.
- An OpenCode worker writes and commits a file. Removal refuses both its dirty
  worktree and its unmerged branch. The live orchestrator then fast-forwards
  the branch and removes the worker, worktree, and branch.
- The real OpenCode harness receives a synthetic HTTP 402 from a loopback API
  fixture, reports the failure to the orchestrator, and becomes idle.
- OpenCode relaunch and stop/start retain the session ID and conversation;
  the worker handles a queued request using a token remembered from before
  restart. The Claude orchestrator also resumes and sends a message.

**Failed: reliable Claude worker resumption.** In both an existing and a fresh
Haiku conversation, a stopped/restarted worker remembers its previous token
but refuses to call Hypercube tools. The native `/mcp` menu shows Hypercube
connected with four tools, while the first resumed API request marks those
tools unavailable. An explicit follow-up asking for ToolSearch can restore
actual calls. SessionStart discovery guidance, per-server `alwaysLoad`, and
`MCP_CONNECTION_NONBLOCKING=0` did not make the queued first-turn test pass;
those experimental changes were not retained. This remains an open integration
issue; the smoke test is not a clean pass.

The automated suite still covers forced waiter replacement, failed writes,
native-subagent hook isolation, stale pane ownership, and launch/session
behavior with local stand-ins. Those tests do not reproduce this native
Claude resume/tool-discovery failure.
