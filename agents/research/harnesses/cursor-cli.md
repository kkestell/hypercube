# Cursor CLI

## 1. Overview
[docs](https://cursor.com/docs/cli/overview) Cursor's terminal coding agent; install with `curl https://cursor.com/install -fsS | bash`. Current version, license, source availability and CLI-specific adoption numbers: unknown. Cursor is a commercial product; do not assume its CLI is open source. No local installation or model calls were made.

## 2. Interactive mode in a PTY
[docs](https://cursor.com/docs/cli/overview) `agent [prompt]` starts a conversational terminal UI with command approvals. Full-screen behavior, resize and xterm.js compatibility: unknown.

## 3. Non-interactive/headless and server modes
[docs](https://cursor.com/docs/cli/reference/parameters) `-p`/`--print`, `--output-format text|json|stream-json`, `--stream-partial-output`; print mode can write files and run shell commands. [docs](https://cursor.com/docs/cli/acp) Native `agent acp` speaks newline JSON-RPC over stdio, including `session/new`, `session/load`, `session/prompt`, streaming updates and permissions. ACP is a separate process mode, not an API attached to an existing TUI.

## 4. Conversation identity
[docs](https://cursor.com/docs/cli/reference/parameters) `agent create-chat` creates a chat and returns its ID; `--resume <chatId>` resumes it; `--continue` selects the last one. [docs](https://cursor.com/docs/agent/hooks) hook input contains stable `conversation_id` and optional `transcript_path`. Caller-chosen IDs, storage location and concurrent same-directory behavior: unknown.

## 5. Hooks and events
[docs](https://cursor.com/docs/agent/hooks) JSON stdin/stdout command hooks include `sessionStart`, `sessionEnd`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `stop`, `subagentStart`, `subagentStop`. `sessionStart` injects context, `postToolUse` returns `additional_context`, and `stop` returns `followup_message`; default stop loop limit is 5 (`loop_limit` configurable). No dedicated permission-wait event identified. Project `.cursor/hooks.json` and `~/.cursor/hooks.json` are shared; [docs](https://cursor.com/docs/cli/reference/parameters) `--plugin-dir <path>` provides a potential per-process route via plugin hooks, but actual CLI hook coverage and idle wake need testing.

## 6. Plugins/extensions
[docs](https://cursor.com/docs/plugins) Cursor plugins bundle hooks, MCP, rules, agents, commands and skills; manifest `.cursor-plugin/plugin.json`. [docs](https://cursor.com/docs/cli/reference/parameters) repeat `--plugin-dir <path>` for local per-process loading. Whether every plugin component operates identically in CLI: unknown.

## 7. MCP
[docs](https://cursor.com/docs/cli/mcp) CLI uses editor config `.cursor/mcp.json` and `~/.cursor/mcp.json`; stdio, HTTP, SSE; `--approve-mcps` approves servers. [docs](https://cursor.com/docs/plugins) plugin may package `mcp.json`; combined with `--plugin-dir` this is a potential process-local configuration, not locally verified. [docs](https://cursor.com/docs/cli/acp) ACP uses configured project/user servers; team MCP is unsupported there.

## 8. Instructions and customization
[docs](https://cursor.com/docs/cli/reference/parameters) `--model`, `--mode plan|ask`, `--force`/`--yolo`, `--sandbox enabled|disabled`, and headless-only `--trust`. [docs](https://cursor.com/docs/plugins) plugin rules, commands, skills and custom agents provide customization. Per-process system prompt flag: unknown.

## 9. Getting messages INTO a running interactive session from outside
[docs](https://cursor.com/docs/agent/hooks) `postToolUse.additional_context` injects during work; `stop.followup_message` continues at completion, subject to loop limit. Idle wake mechanism into an existing TUI: unknown; PTY typing is a fallback and can interfere with user input. [docs](https://cursor.com/docs/cli/acp) ACP `session/prompt` targets an ACP session, not an already-running terminal UI.

## 10. Status detection from outside
[docs](https://cursor.com/docs/agent/hooks) prompt/tool/stop hooks can approximate busy and completion; permission-wait detection in TUI is unknown. [docs](https://cursor.com/docs/cli/acp) ACP emits `session/update`, `session/request_permission`, and blocking `cursor/ask_question`/`cursor/create_plan`; pending requests expose attention state.

## 11. Usage and cost reporting
Unknown for CLI event or hook output; do not infer availability from account billing.

## 12. Gotchas for running many instances side by side
[docs](https://cursor.com/docs/cli/mcp) MCP approvals and configuration are shared with editor; [docs](https://cursor.com/docs/agent/hooks) matching hooks from all sources run and responses merge. Same-chat locking, shared auth limits and rate limits: unknown. Avoid `--continue` for independent instances.

## 13. Fit for Hypercube
- [docs](https://cursor.com/docs/cli/reference/parameters) Launch a PTY with an explicit model and a per-process `--plugin-dir`; validate hook and MCP loading in CLI.
- [docs](https://cursor.com/docs/agent/hooks) Track `conversation_id` and resume explicitly; inject queued work via tool/stop hooks while active.
- Idle wake and reliable permission detection for an existing interactive process remain unknown; avoid claiming stop follow-up is an idle wake.
- [docs](https://cursor.com/docs/cli/acp) ACP is a strong alternative for machine control but requires a separate client UI rather than attaching to the CLI's TUI.
