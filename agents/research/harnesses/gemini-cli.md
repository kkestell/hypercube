# Gemini CLI (`gemini`)

Research date: 2026-10-29. **Not installed on this machine** (`which gemini` → not found) and I did not install it. Every behavior claim below is **[docs]** from the repo's `main` branch, which can be ahead of the latest release (0.66.0). Flags and file names may differ in the release you install. Check against the installed version before building on any of them.

Tags: **[verified]** = checked against a registry or the GitHub API from this machine (no install). **[docs Gn]** = from the official repo docs or source listed in Sources. "unknown" = not found in the docs I read.

## 1. Overview

- Google's terminal coding agent for Gemini models. Open source.
- License: Apache-2.0 [verified: `npm view @google/gemini-cli license`; GitHub API `license.spdx_id`].
- Current versions [verified: `npm view @google/gemini-cli dist-tags`]: `latest` 0.66.0; `preview` 0.67.0-preview.0; `nightly` 0.67.0-nightly.20261028.
- Install: `npm install -g @google/gemini-cli` [docs G14]. npm `bin` is `gemini` [verified].
- Adoption: npm 449,910 downloads for 2026-10-22..10-28 [verified: npm downloads API]. GitHub `google-gemini/gemini-cli` 108,612 stars [verified: `gh api`].

## 2. Interactive mode in a PTY

- `gemini` with no prompt starts an interactive REPL [docs G4]. A positional query defaults to interactive when a TTY is attached [docs G4].
- `-i/--prompt-interactive "query"` runs a prompt and then stays interactive [docs G4].
- `--screen-reader` enables an accessibility mode [docs G4].
- Full-screen or alternate-screen behavior: unknown (not installed).
- Behavior under xterm.js and resize: unknown (not installed).
- Terminal notifications: experimental. When action is required (tool approval) or a session completes, Gemini sends OSC 9 (falls back to BEL) [docs G9]. Off by default; enable with `general.enableNotifications` or `/settings` [docs G9, G5].

## 3. Non-interactive and server modes

- Headless is triggered by a non-TTY environment or by `-p/--prompt` [docs G2]. `-p` text is appended to stdin input if stdin is provided [docs G4].
- `-o/--output-format text|json|stream-json` (default `text`) [docs G4].
- JSON output is one object: `response` (string), `stats` (token usage and API latency), `error` (optional) [docs G2]. The docs do not list a session id in this object.
- stream-json emits newline-delimited events: `init` (session metadata: session id, model), `message`, `tool_use`, `tool_result`, `error`, `result` (aggregated stats and per-model token usage) [docs G2].
- Exit codes: 0 success; 1 general or API failure; 42 input error; 53 turn limit exceeded [docs G2].
- **ACP natively: yes.** `gemini --acp` starts Agent Client Protocol mode over stdio JSON-RPC 2.0 [docs G7]. The CLI reference lists `--experimental-acp`, marked experimental [docs G4]. The source has both `--acp` (used by integration tests) and an `experimental-acp` option [verified: `gh search code` in google-gemini/gemini-cli]. Treat the flag name as unresolved until checked on the installed version.
- ACP methods [docs G7]: `initialize` (lets the client register its MCP server), `authenticate`, `newSession`, `loadSession`, `prompt`, `cancel`, `setSessionMode`, `unstable_setSessionModel`. The agent sends `session/request_permission` to the client for tool approval. File reads and writes are proxied through the client [docs G7].
- Gemini CLI is listed in the ACP Agent Registry for Zed and JetBrains [docs G7].
- Server or HTTP API mode: none documented.

## 4. Conversation identity

- **Caller-chosen id:** not documented. The CLI reference has no session-id flag [docs G4]. Unknown whether ACP `newSession` accepts one.
- **Learning the id:** `init` event in stream-json includes the session id [docs G2]. ACP `newSession` returns a session identifier, but the response shape is not documented in the page I read. Unknown.
- **Resume:** `-r latest` (most recent), `-r <index>` (from `--list-sessions`), `-r <full session UUID>`. `-r "<id>" "query"` resumes and sends a new prompt [docs G4, G3]. In the TUI, `/resume` opens the session browser [docs G3]. ACP: `loadSession` [docs G7].
- Session management: `--list-sessions`, `--delete-session <index|id>` [docs G4].
- **Storage:** `~/.gemini/tmp/<project_hash>/chats/`. `<project_hash>` is derived from the project root [docs G3]. File format: not documented.
- **Scope:** sessions are per project. Switching to another project switches the history [docs G3].
- **Several instances in one directory:** `-r latest` is ambiguous when several sessions exist. Resume by full id. Whether two live processes can write one chat file at the same time: unknown.
- **Retention (important for long-lived sessions):** `general.sessionRetention` defaults to `enabled: true`, `maxAge: "30d"`. Sessions older than 30 days are deleted automatically. `maxCount` defaults to unlimited. `minRetention` defaults to `"1d"` [docs G3]. Delete manually with `gemini --delete-session <index or ID>` or from the Session Browser [docs G3]. Set `enabled: false` or a longer `maxAge` for sessions you keep.

## 5. Hooks and events

- **Config:** `settings.json` has a `hooks` object keyed by event name. Each event holds an array of `{ "matcher": "…", "sequential": false, "hooks": [ { "type": "command", "command": "…", "name": "…", "timeout": 60000, "description": "…" } ] }` [docs G1]. `type` supports only `"command"` [docs G1]. `timeout` is in milliseconds, default 60000 [docs G1]. `matcher` is a regex for tool events and an exact string for lifecycle events [docs G1].
- **Events** [docs G1]: `BeforeTool`, `AfterTool`, `BeforeAgent` (after prompt submit, before planning), `AfterAgent` (once per turn, after the final response), `BeforeModel`, `BeforeToolSelection`, `AfterModel`, `SessionStart` (`source`: `startup|resume|clear`), `SessionEnd` (`reason`: `exit|clear|logout|prompt_input_exit|other`; the CLI does not wait for it), `Notification` (`notification_type`: `ToolPermission` only; observability only), `PreCompress` (`trigger`: `auto|manual`).
- **Stdin base fields** [docs G1]: `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `timestamp`. Event-specific fields: `BeforeAgent` has `prompt`; `AfterAgent` has `prompt`, `prompt_response`, `stop_hook_active`; `BeforeTool`/`AfterTool` have `tool_name`, `tool_input` (and `tool_response` after).
- **Exit codes** [docs G1]: 0 = success, stdout must be only the JSON object. 2 = block; stderr is the reason. Other = warning, action continues.
- **JSON output** [docs G1]: `decision` (`allow|deny`; `block` is an alias), `reason`, `systemMessage`, `continue`, `stopReason`, `suppressOutput`, `hookSpecificOutput.additionalContext`.
- **Turn end:** on `AfterAgent`, `decision: "deny"` rejects the response and starts a retry with `reason` sent as a new prompt. Exit 2 also retries, with stderr as the prompt [docs G1]. `continue: false` stops the session without retrying [docs G1].
- `SessionStart` additionalContext: injected as the first turn in interactive mode, prepended to the prompt in non-interactive mode [docs G1]. `SessionStart`, `SessionEnd`, `Notification`, `PreCompress` ignore `continue` and `decision` [docs G1].
- **Environment variables for hooks:** none documented in the reference [docs G1].
- **Per-process injection: not documented.** Hooks come from settings files only [docs G1]. Settings locations: user `~/.gemini/settings.json`, workspace `.gemini/settings.json` [docs G5]. The settings page does not list a flag or env var that points the CLI at another settings file or home directory. Unknown.

## 6. Plugins and extensions

- Extension = directory with a `gemini-extension.json` manifest in its root [docs G10]. Manifest fields seen in the reference: `name`, `version`, `mcpServers`, `contextFileName` (default `GEMINI.md`), `excludeTools`, `settings` [docs G10].
- Extensions bundle prompts, MCP servers, custom commands, themes, hooks, sub-agents, and agent skills [docs G10 index].
- Install and manage [docs G4, G10]: `gemini extensions install <git URL or local path> [--ref <ref>] [--auto-update] [--pre-release] [--consent] [--skip-settings]`; `uninstall`, `list`, `update <name>|--all`, `enable`, `disable`, `link <path>` (dev link), `new <path>`, `validate <path>`.
- **Per-run selection:** `-e/--extensions <list>`; if omitted, all installed extensions are enabled [docs G4]. `-l/--list-extensions` lists them [docs G4].
- Loading an unpacked extension for one run without installing it: unknown. `link` and `install` are persistent.
- A `settings` entry in `mcpServers` inside an extension is overridden by the same server name in `settings.json` [docs G10].

## 7. MCP

- Config location: `mcpServers` in `~/.gemini/settings.json` (user) or `.gemini/settings.json` (workspace) [docs G5, G11].
- Transports [docs G11]: stdio via `command`, `args`, `env`, `cwd`; SSE via `url`; streamable HTTP via `httpUrl`. WebSocket: not documented.
- CLI: `gemini mcp add <name> <command|url> [-s user|project] [-t stdio|sse|http] [-e KEY=value] [-H header] [--timeout ms] [--include-tools] [--exclude-tools] [--trust]`; also `list`, `remove`, `enable`, `disable` (`--session` for session-only) [docs G11]. `add` writes to the chosen settings scope; `--trust` bypasses tool-call confirmation for that server [docs G11].
- Per-run: `--allowed-mcp-server-names <list>` restricts which configured servers connect [docs G4]. In settings, `mcp.allowed` is an allow-list of server names and `mcp.excluded` a deny-list [docs G11].
- **Per-process MCP config file or flag: not documented.** Hypercube's per-agent `hypercube` server cannot be passed on the command line the way Claude's `--mcp-config` allows. Unknown whether an env var can point to another settings file.

## 8. Instructions and customization

- **System prompt, per process:** `GEMINI_SYSTEM_MD=<path to .md>` replaces the core system prompt entirely (full replacement, not a merge) [docs G6]. `GEMINI_WRITE_SYSTEM_MD` writes the default out for editing [docs G6].
- **Context files:** `GEMINI.md` is the default name. Scanned in `~/.gemini/GEMINI.md`, the project, and subdirectories. The name can be changed with `contextFileName` in settings or an extension manifest [docs G13, G10].
- **Model:** `-m/--model` takes `auto` (default), `pro`, `flash`, `flash-lite`, or a full model name [docs G4].
- **Approval mode, per process:** `--approval-mode default|auto_edit|yolo|plan` [docs G4]. `-y/--yolo` is deprecated in favor of `--approval-mode=yolo` [docs G4]. `--allowed-tools` is deprecated in favor of the Policy Engine [docs G4].
- `-s/--sandbox` runs in a sandbox [docs G4]. `--include-directories <list>` adds workspace directories [docs G4]. `-w/--worktree` requires `experimental.worktrees` [docs G4].
- Custom slash commands, skills, and agents: reload commands `/commands reload`, `/skills reload`, `/agents reload` [docs G4]. Subagents are documented at `docs/core/subagents.md` (not read).
- Plan mode: `general.plan.enabled`, `general.plan.directory`, `general.plan.modelRouting` [docs G5].

## 9. Getting messages into a running interactive session

- No documented API for a running TUI session. Typing into the PTY is the only generic path (not tested).
- The programmatic path is ACP `prompt` on a `gemini --acp` process [docs G7]. That process has no TUI.
- Hooks can only feed text back at turn end: `AfterAgent` deny or exit 2 starts a retry with that text [docs G1]. It cannot wake an idle session.

## 10. Status detection from outside

- Hooks [docs G1]: `BeforeAgent` marks a turn start; `AfterAgent` marks a turn end; `Notification` with `notification_type: "ToolPermission"` marks a permission wait; `SessionEnd` marks exit. All are settings-file hooks (see §5 on per-process injection).
- OSC 9 terminal notifications, experimental and opt-in, for "action required" and "session complete" [docs G9]. Usable by parsing the PTY stream if enabled.
- ACP: `session/request_permission` is an explicit "waiting for approval" request from the agent [docs G7]. Session update events: not read.
- Headless: stream-json events [docs G2].
- Busy/idle for a TUI process without hooks or OSC 9: unknown.

## 11. Usage and cost

- Headless JSON: `stats` (token usage and API latency) [docs G2]. stream-json `result`: aggregated stats and per-model token usage [docs G2].
- Telemetry (OpenTelemetry), per-process env vars [docs G8]: `GEMINI_TELEMETRY_ENABLED` (true/false, default false), `GEMINI_TELEMETRY_OUTFILE` (write to a file; overrides the OTLP endpoint), `GEMINI_TELEMETRY_OTLP_ENDPOINT`, `GEMINI_TELEMETRY_OTLP_PROTOCOL`, `GEMINI_TELEMETRY_OTLP_HEADERS`, `GEMINI_TELEMETRY_TARGET`, `GEMINI_TELEMETRY_LOG_PROMPTS`, `GEMINI_TELEMETRY_TRACES_ENABLED`. Metrics include `gemini_cli.token.usage` and `gen_ai.usage.input_tokens` / `output_tokens` [docs G8].
- Dollar cost: not documented (unknown).

## 12. Gotchas for many instances

- **Settings are shared files.** Hooks, MCP servers, and extensions all live in `~/.gemini/settings.json` or the workspace `.gemini/settings.json` [docs G5, G11]. Concurrent writes from many instances are not addressed in the docs (unknown).
- **Chat history is per project path hash** under `~/.gemini/tmp/` [docs G3]. Moving a project directory changes its hash, so history no longer matches.
- **Retention deletes old sessions by default** (`maxAge` 30 days) [docs G3]. Long-lived sessions need the setting changed.
- **Folder trust is off by default** [docs G12]. When turned on, untrusted folders behave differently. Whether project-level hooks run in an untrusted folder: unknown. `--skip-trust` trusts the current workspace for one run [docs G4].
- **Experimental features:** notifications [docs G9]; worktrees (`-w`) [docs G4]; ACP flag name (see §3).
- **Docs are from `main`,** not the 0.66.0 release. Cross-check flags against `gemini --help` on the installed version.
- Deprecated: `-y/--yolo`, `--allowed-tools` [docs G4].
- Telemetry: the outfile setting overrides the OTLP endpoint [docs G8].

## 13. Fit for Hypercube

- **Launch:** TUI per agent, with per-process `-m`, `--approval-mode`, `-e`, `--allowed-mcp-server-names`, `--include-directories`, and `GEMINI_SYSTEM_MD`. The gap is MCP and hooks: both must be written into settings files, so per-agent wiring (Hypercube MCP server with `HYPERCUBE_AGENT_ID`, status hooks) would need per-agent settings files or an unknown override.
- **Possible alternative:** run agents as `gemini --acp` and drive `newSession` / `loadSession` / `prompt` / `cancel` over stdio. That gives explicit session control and explicit permission requests, but there is no TUI for the user to see, so it changes the product surface. Needs a separate decision.
- **Conversation tracking:** no caller-chosen id. Capture the id from stream-json `init` (headless) or the ACP `newSession` response (shape not confirmed). Resume with `-r <id>` or `loadSession`. Set retention so sessions survive.
- **Status:** `BeforeAgent`/`AfterAgent` hooks and `ToolPermission` notifications, all via settings files. ACP's `session/request_permission` is the most explicit waiting signal. OSC 9 is opt-in and only covers approvals and completion.
- **Message delivery:** ACP `prompt` is the only documented injection API. The TUI has none, and the `AfterAgent` retry works only at turn end.
- **Missing:** a per-process hook or MCP config, a caller-chosen session id, a per-process settings home, a documented busy/idle signal for the TUI, and any test on an installed binary.

## Unknown (not checked)

- Gemini is not installed, so TUI rendering, resize, xterm.js behavior, and every flag were checked only against docs.
- Whether `--acp` or `--experimental-acp` is the correct flag on the release.
- ACP `newSession` response shape (session id field).
- Whether an env var or flag points the CLI at another settings file or home directory.
- Per-process MCP config or hook injection without a settings file.
- Whether project-level hooks run in untrusted folders.
- Whether a `gemini --acp` session can be started with a caller-chosen id.
- Dollar cost reporting.
- Whether two live processes can safely share one project's chat directory.

## Sources

- G1 https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/reference.md
- G2 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md
- G3 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/session-management.md
- G4 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md
- G5 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/settings.md
- G6 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/system-prompt.md
- G7 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md
- G8 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/telemetry.md
- G9 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/notifications.md
- G10 https://github.com/google-gemini/gemini-cli/blob/main/docs/extensions/reference.md and https://github.com/google-gemini/gemini-cli/blob/main/docs/extensions/index.md
- G11 https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md
- G12 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/trusted-folders.md
- G13 https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/gemini-md.md
- G14 https://github.com/google-gemini/gemini-cli/blob/main/docs/get-started/index.md
- G15 https://www.npmjs.com/package/@google/gemini-cli
- Source check: `gh search code "experimental-acp"` and `gh search code "'--acp'"` in google-gemini/gemini-cli (`packages/cli/src/config/config.ts`, `integration-tests/`).
- Raw files were read with `curl` from `raw.githubusercontent.com/google-gemini/gemini-cli/main/docs/…`.
