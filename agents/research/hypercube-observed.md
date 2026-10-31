# Harness behavior observed in Hypercube

Facts established by Hypercube's own integration, which runs every agent in
the user's workspaces, as of 2026-10-30. Sources are the launch code in
`internal/harness/harness.go`, the OpenCode plugin in
`internal/harness/opencode.js`, `docs/harness-support.md`, and what happened
in use. Versions: Claude Code 2.1.314, OpenCode 2.1.4.

## Claude Code, on the user's Claude Team login

- Every agent launches with `--mcp-config <inline JSON>`, `--settings <inline
  JSON>` holding hooks and a status line, `--model`, `--effort`, and
  `--permission-mode auto`. All of these work on the Team login; managed
  settings do not block `--mcp-config` or `--settings` hooks for this org.
- Hooks configured through `--settings`: `SessionStart`, `UserPromptSubmit`,
  `PostToolUse`, `Stop`, `StopFailure`, `PermissionRequest`,
  `PostToolUseFailure`, and `PermissionDenied`. `SessionStart`
  `additionalContext`, `UserPromptSubmit` and `PostToolUse` context, and
  `Stop` with `{"decision": "block", "reason": …}` all deliver text.
- Idle wake works: `SessionStart`, `Stop`, and `StopFailure` also run a
  waiter as a command hook with `"asyncRewake": true` and
  `"timeout": 1728000` (20 days). The waiter blocks until messages arrive,
  writes them to stderr, and exits 2, which starts a turn in the idle
  interactive session. Hypercube's documentation states that this leaves the
  user's unsent draft in its editor.
- The session ID is learned from `session_id` in `UserPromptSubmit`; Claude
  saves no conversation before the first prompt, so an ID seen earlier may
  not be resumable. Resume is `--resume <id>`, with every flag passed again.
- No hook fires when the user interrupts a turn; the agent stays busy in
  Hypercube's state until another prompt or completed turn.
- A resumed worker can remember its conversation but refuse Hypercube's MCP
  tool calls: `/mcp` shows the tools connected while the resumed request
  marks them unavailable. Asking the agent to use ToolSearch restores them.
- `--disallowedTools` takes a list of tools, so it consumes every following
  argument that is not a flag. Placed just before the positional prompt, it
  took the prompt's words as tool names and the agent started with no prompt.
  `--disallowedTools Agent` must be followed by another flag.
- The status line command receives the session's model, effort, context
  window use, and rate limits as JSON. Claude redraws it after an approval
  dialog closes, which Hypercube uses to clear its attention bell.
- A worker launched in a directory Claude has not trusted (for example a new
  folder under `/tmp`) stops at Claude's folder trust dialog ("Is this a
  project you created or one you trust?") before any hook runs, so no prompt
  is submitted and Hypercube sees no signal. Worktrees inside a trusted
  checkout do not hit it.
- `PermissionRequest` fires for native subagents' approvals too, with an
  `agent_id` field identifying the subagent.

## OpenCode 2.1.4

- Every agent launches as
  `opencode --standalone --auto --session <id> [--prompt <text>]`, with:
  - `OPENCODE_CONFIG_CONTENT` setting `model`, the `build` agent's
    `model` as `<model>#<variant>`, `permissions` rules, `mcp.servers`, and
    `plugins`. OpenCode 2.1.4 honors all of these.
  - `OPENCODE_CLI_CONFIG_CONTENT` set to `{"tabs":{"mode":"off"}}`, which
    turns session tabs off so the pane stays on its conversation.
- A caller-chosen session ID works: `ses_hypercube_<agent id>`. The session
  exists only after its first prompt.
- Code Mode lists OpenCode's own tools but not MCP tools; Hypercube turns it
  off with `"codemode": false` on the MCP server and a permission rule
  `{"action": "execute", "resource": "*", "effect": "deny"}` on the `build`
  agent.
- Native subagents are denied with
  `{"action": "subagent", "resource": "*", "effect": "deny"}`, the rule
  OpenCode's own built-in subagents carry (`opencode debug agents`). That
  `debug agents` does not read `OPENCODE_CONFIG_CONTENT`, so the effect of the
  rule in a launched agent has not been checked separately.
- With an unavailable model ID, OpenCode silently falls back to a recent
  model. Hypercube's plugin compares the session's selected model on the
  first prompt and rejects the prompt when it differs.
- The plugin uses `ctx.session.hook("prompt")` to append context to a
  prompt, `ctx.tool.hook("execute.after")` to append context to tool
  results, `ctx.event.subscribe` for `session.execution.succeeded`,
  `.failed`, and `.interrupted`, `ctx.session.prompt` to start a turn, and
  `ctx.session.get` to find a session's parent. The event stream fails when
  its consumer falls behind, so the plugin subscribes only to turn ends and
  resubscribes a second after the stream ends.
- OpenCode's model catalog is cached in `~/.cache/opencode/models.json` and
  refreshed by the background service. A long-running service kept a
  five-day-old catalog that lacked models the live catalog at models.dev
  listed.

## Codex

- Hypercube's first Go version (`930027c^:internal/harness/harness.go`)
  launched Codex with `codex [resume] --dangerously-bypass-approvals-and-sandbox
  --dangerously-bypass-hook-trust`, and `-c` overrides for `model`,
  `model_reasoning_effort`, `mcp_servers.hypercube={command=…, args=["mcp"],
  env={…}}`, `projects={"<main checkout>"={trust_level="trusted"}}`, and
  `hooks.<Event>=[{hooks=[{type="command", command=…}]}]` for
  `SessionStart`, `UserPromptSubmit`, `PostToolUse`, and `Stop`. That
  targeted the Codex release of 2026-10-25 to 2026-10-27; whether the hooks
  fired in a live run is not recorded. It woke idle agents by typing into the
  tmux pane.
