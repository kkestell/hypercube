# OpenAI Codex CLI

## 1. Overview
[docs](https://github.com/openai/codex) OpenAI's local coding agent is open source under Apache-2.0; repository shows about 132k stars. Install via `npm install -g @openai/codex` or `brew install --cask codex`. [verified] Installed version `codex-cli 0.164.0`; latest release unknown.

## 2. Interactive mode in a PTY
[verified] `codex [PROMPT]` launches interactive mode; `--no-alt-screen` chooses inline mode preserving terminal scrollback. xterm.js compatibility and resize behavior unknown, not tested.

## 3. Non-interactive/headless and server modes
[verified] `codex exec [PROMPT]` accepts stdin; `--json` emits JSONL, `--output-last-message FILE` writes final answer, `--ephemeral` avoids session persistence. [docs](https://developers.openai.com/codex/app-server/) `codex app-server` speaks JSON-RPC over stdio, Unix socket, or experimental WebSocket; TUI can connect with `codex --remote`. ACP native support unknown; adapter availability unknown.

## 4. Conversation identity
[verified] `codex resume SESSION_ID [PROMPT]` resumes UUID or name; `--last` selects newest. Caller-chosen new ID unknown. [docs](https://developers.openai.com/codex/app-server/) `thread/start` returns `thread.id`, `thread/resume` accepts it; `thread.sessionId` is tree-root identity, which can differ for forks. Persistent histories use paginated storage by default; exact paths and simultaneous same-directory behavior unknown. Hooks include `session_id` and `transcript_path` [docs](https://developers.openai.com/codex/hooks/).

## 5. Hooks and events
[docs](https://developers.openai.com/codex/hooks/) Events: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Stop`, `SessionEnd`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Interrupt`. Configure `~/.codex/hooks.json`, `<repo>/.codex/hooks.json`, or inline `[hooks]` in config; command hooks consume JSON stdin. `SessionStart`, prompt and post-tool can add `hookSpecificOutput.additionalContext`; `Stop` may return `decision: "block"` and `reason` to continue. Hooks require trust; [verified] `--dangerously-bypass-hook-trust` bypasses it. Per-process hook injection using `-c` is plausible from config syntax but unverified; unknown whether overrides are discovered as hooks. Async hooks cannot wake an idle session: output waits for next user turn [docs](https://developers.openai.com/codex/hooks/).

## 6. Plugins/extensions
[docs](https://developers.openai.com/codex/hooks/) Plugins may bundle hooks via `.codex-plugin/plugin.json` or `hooks/hooks.json`, and undergo hook trust review. [verified] `codex plugin add/list/remove` manages marketplace plugins. Per-process plugin loading unknown.

## 7. MCP
[docs](https://developers.openai.com/codex/mcp/) MCP client supports stdio and streamable HTTP; config under `mcp_servers`. [verified] `-c key=value` overrides TOML config per process; passing an MCP table via `-c` is untested.

## 8. Instructions and customization
[docs](https://developers.openai.com/codex/app-server/) `thread/start` and `turn/start` support configuration and instruction overrides; skills and subagents supported. [verified] `-m MODEL`, `-c key=value`, `-s read-only|workspace-write|danger-full-access`, `-a on-request|never`, `--oss`, `--local-provider`; `--dangerously-bypass-approvals-and-sandbox` disables safeguards. Exact per-process system prompt and custom command flags unknown.

## 9. Getting messages INTO a running interactive session from outside
[verified] `codex queue --thread ID --message TEXT` targets a session; behavior for idle sessions and non-daemon sessions untested. [docs](https://developers.openai.com/codex/app-server/) Shared app-server supports `turn/steer` during a turn, `turn/start` while idle, and `thread/inject_items`; remote TUI can connect to that server. Whether an independently launched TUI is attached to a controllable server and preserves typed input needs testing. Async hooks do not awaken an idle turn [docs](https://developers.openai.com/codex/hooks/).

## 10. Status detection from outside
[docs](https://developers.openai.com/codex/app-server/) `thread/status/changed` has `idle`, `active`, `notLoaded`, `systemError`; active flags include `waitingOnApproval`. `turn/started`, `turn/completed`, `item/started`, `item/completed`, and approval server requests provide finer signals. User-input request uses `tool/requestUserInput`. Availability when observing a separate existing TUI needs testing.

## 11. Usage and cost reporting
[docs](https://developers.openai.com/codex/app-server/) App-server provides `thread/tokenUsage/updated` and account rate-limit information. Dollar cost reporting unknown.

## 12. Gotchas for running many instances side by side
[verified] Default CLI can use a shared local daemon; `--no-daemon` disables it. [docs](https://developers.openai.com/codex/app-server/) WebSocket transport is experimental; non-loopback listeners may default to unauthenticated, so configure auth and TLS. Shared account rate limits, state contention and concurrent same-thread semantics unknown.

## 13. Fit for Hypercube
- [verified] Launch interactive `codex`, optionally `--remote`, and resume by saved UUID.
- [docs](https://developers.openai.com/codex/app-server/) App-server offers structured status, approval requests, live steering and new turns; requires ownership/connectivity architecture validation.
- [docs](https://developers.openai.com/codex/hooks/) Hooks provide familiar stop-block and context delivery, but async completion does not wake idle sessions.
- [verified] `codex queue` is promising for external delivery; idle wake and queue/TUI interaction remain untested.
