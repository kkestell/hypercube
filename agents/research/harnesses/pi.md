# Pi

## 1. Overview
[verified] Installed `pi --version` is `1.4.0`; package metadata identifies `@earendil-works/pi-coding-agent`, author Mario Zechner, MIT license. [docs](https://github.com/earendil-works/pi) Open-source Pi toolkit; repository shows approximately 119k stars, a toolkit-wide rather than CLI-specific adoption signal. Install via `npm install -g --ignore-scripts @earendil-works/pi-coding-agent` or official installer. No model calls made.

## 2. Interactive mode in a PTY
[verified] `pi --help` says `--tui-mode fullscreen` is default, with `regular` available. Resize and xterm.js compatibility: unknown; not exercised.

## 3. Non-interactive/headless and server modes
[verified] `--print`/`-p`, `--mode text|json|rpc`. [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md) RPC is a separate long-lived subprocess using newline JSON commands on stdin and events on stdout, not a control socket for a TUI. `prompt` acceptance does not mean completion; wait for `agent_settled`. Native ACP or adapter: unknown. [docs](https://github.com/earendil-works/pi) TypeScript SDK available.

## 4. Conversation identity
[verified] `--session-id <id>` uses exact project session ID, creating it if missing; `--session <path|id>` resolves a path or partial UUID; `--session-dir <dir>`, `--continue`, `--resume`, `--no-session`. Default session directory shown in help is `~/.pi/agent/sessions/--path--/`; `PI_CODING_AGENT_SESSION_DIR` overrides it. [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) extensions can read `ctx.sessionManager.getSessionId()`. Same session concurrently opened by multiple instances: unknown; use distinct IDs.

## 5. Hooks and events
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) Extensions subscribe via `pi.on` to `session_start`, `session_shutdown`, `input`, `before_agent_start`, `agent_start`, `agent_end`, `agent_before_settle`, `agent_settled`, `tool_call`, `tool_result` and tool execution events. `before_agent_start` can return a message; `tool_result` can replace content; `agent_settled` occurs after automatic retry/compaction/queued work, unlike `agent_end`. [verified] `--extension <path>` injects per process; `--no-extensions` still permits explicitly loaded paths. Tool-permission prompt event: unknown; Pi has no built-in permission system.

## 6. Plugins/extensions
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) JS/TS extension default export receives `pi`; can register tools, commands, UI, events, modify prompts/results, send messages and register MCP servers. [verified] Repeat `--extension <path>` for process-local loading; `--skill` and `--prompt-template` also load per process.

## 7. MCP
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md) Built-in MCP supports stdio and streamable HTTP, user `~/.pi/agent/mcp.json`, trusted project `.pi/mcp.json`. Extension `pi.registerMcpServer(name, {command,args,env,exposure})` registers per session without changing config. A same-name `mcp.json` entry takes precedence. [verified] `--no-mcp` disables built-in MCP.

## 8. Instructions and customization
[verified] `--system-prompt`, repeatable `--append-system-prompt`, `--model provider/id`, `--provider`, `--thinking off|minimal|low|medium|high|xhigh|max`, `--tools`, `--exclude-tools`, `--approve` or `--no-approve` for project trust. [docs](https://github.com/earendil-works/pi) Pi has no built-in filesystem/process/network permission system and skips built-in subagents; extensions may implement them. Prompt templates and extensions supply commands. Project trust is not a sandbox.

## 9. Getting messages INTO a running interactive session from outside
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) An extension can use `pi.sendUserMessage(text)` to start a run and inject context with `before_agent_start`/`tool_result`. A process-local extension could receive an external signal or wait on a child process then call this API; exact idle-wakeup and editing-interference behavior must be tested. RPC `prompt` controls its own subprocess, not another interactive session.

## 10. Status detection from outside
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) Extension can relay `before_agent_start`, `agent_settled` and `session_shutdown`; `agent_end` alone can be premature. [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md) RPC exposes streamed events and settlement. Built-in tool permission waiting does not apply; waiting on extension UI/user questions needs separate detection (unknown).

## 11. Usage and cost reporting
[docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/session-format.md) Session records include assistant message usage; exact provider accounting and external live billing limits: unknown.

## 12. Gotchas for running many instances side by side
[verified] Default credentials and settings live under `~/.pi/agent`; `PI_CODING_AGENT_DIR` changes config directory. [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md) same-name MCP config overrides extension registration and OAuth credentials are shared by server name and URL. Session locking, provider rate limits, and concurrent shared session safety: unknown. Project trust and permission isolation are distinct.

## 13. Fit for Hypercube
- [verified] Launch `pi --extension <path> --model <provider/id> --thinking <level> --approve --session-id <id> [prompt]`; explicit ID enables resume without guessing.
- [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) Register Hypercube MCP and relay lifecycle hooks through a per-process extension, accounting for config name collisions.
- [docs](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) Deliver in-flight context at tool results and stop at `agent_settled`; an extension-owned external waiter can potentially wake idle sessions, but needs a behavioral test.
- Permission status is not a normal built-in Pi concept; UI prompts from extensions and concurrent session safety need separate investigation.
