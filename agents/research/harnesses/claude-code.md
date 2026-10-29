# Claude Code (`claude`)

Research date: 2026-10-29. Local binary: `claude` 2.1.312, installed (native installer, `~/.local/bin/claude` → `~/.local/share/claude/versions/2.1.312`).

Tags: **[verified]** = run locally on this machine. **[docs Sn]** = from the official docs or registry page listed in Sources. Anything not checked is written as "unknown".

## 1. Overview

- Anthropic's terminal coding agent. Interactive TUI, print mode, Agent SDK, MCP client and server, plugins, hooks.
- License: not an OSI open-source license. npm metadata says `SEE LICENSE IN README.md` [verified: `npm view @anthropic-ai/claude-code license`]. GitHub reports no SPDX license for `anthropics/claude-code` [verified: `gh api`]. I did not read the README's terms.
- Current version: 2.1.312 [verified: `claude --version`]; npm `version` is also 2.1.312 [verified].
- Install methods: native installer `curl -fsSL https://claude.ai/install.sh | bash`; Homebrew cask `claude-code`; WinGet `Anthropic.ClaudeCode` [docs S13]. Native installs auto-update; Homebrew and WinGet do not [docs S13].
- Adoption: npm `@anthropic-ai/claude-code` 12,837,062 downloads for 2026-10-22..10-28 [verified: npm downloads API]. GitHub `anthropics/claude-code` 153,839 stars [verified: `gh api`].

## 2. Interactive mode in a PTY

- **Full-screen?** Not by default. In a 10-second startup capture at 120x40 with default settings and a fresh `CLAUDE_CONFIG_DIR`, the TUI did **not** enter the alternate screen (no `ESC[?1049h`) [verified: PTY probe].
- Modes it turned on at startup: bracketed paste `ESC[?2004h`, focus reporting `ESC[?1004h`, hide cursor `ESC[?25l` [verified]. Not seen in that capture: mouse tracking (`?1000h`, `?1006h`), kitty keyboard push (`ESC[>1u`), synchronized output (`?2026h`) [verified].
- The fresh-config startup showed the welcome/onboarding screen. A `SessionStart` hook did not fire within 10 s of TUI startup on a fresh config, although it did fire in print mode (see §5) [verified]. Hypercube's TUI launch will need onboarding and trust already accepted for the agent's config dir, or a documented way to skip them. I did not find a per-process flag for that; unknown.
- Fullscreen rendering is a research preview. The start mode depends on the user's `tui` setting; `/tui fullscreen` switches it; `CLAUDE_CODE_NO_FLICKER=1` forces fullscreen; `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN=1` forces the classic main-screen renderer [docs S12, S4]. Attached background sessions always render fullscreen [docs S6]. Setting one of these env vars per process pins the mode regardless of the user's settings.
- **Resize:** works. With the PTY as the controlling terminal, changing the window to 80x24 made claude redraw: 1,907 bytes in the 4 s after the resize [verified: resize probe].
- **xterm.js:** unknown. Not tested in a web terminal.
- Terminal title: Claude sets it. `CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1` turns that off [docs S7].
- Claude Code also reports per-session status via OSC 7501 (§10).

## 3. Non-interactive and server modes

- `-p/--print`, with `--output-format text|json|stream-json`, `--input-format text|stream-json` (realtime streaming input, print only), `--include-partial-messages`, `--include-hook-events` (stream-json only), `--replay-user-messages`, `--json-schema`, `--max-budget-usd` [verified: `claude --help`].
- `claude -p` with empty stdin and no prompt exits 1 with `Error: Input must be provided either through stdin or as a prompt argument when using --print`, with no model call [verified].
- JSON output carries `session_id`, `total_cost_usd`, and a per-model cost breakdown. With `--resume`/`--continue`, the cost is the conversation's total [docs S2]. With `--json-schema`, the result is in `structured_output` [docs S2].
- stream-json: first event is `system/init` with session metadata, MCP and plugin status (`mcp_servers`, `mcp_server_errors`, `plugins`, `plugin_errors`); `system/api_retry` events; the last line is a `result` message [docs S2].
- Exit codes: 0 on success, non-zero on failure [docs S2]. SIGTERM ends a `-p` run with exit 143 and leaves the turn unfinished; SIGINT ends the turn. SIGTERM kills the Bash process tree and runs `SessionEnd` hooks [docs S2].
- Keep-alive for stream-json stdin input: partly described in the docs; not tested. Unknown whether a `-p --input-format stream-json` process stays alive across turns.
- **ACP natively:** no. `claude --help` lists no ACP option [verified]; the docs I read do not mention ACP [docs S2].
- **ACP via adapter:** `@zed-industries/claude-code-acp`, version 0.16.2, Apache-2.0, "An ACP-compatible coding agent powered by the Claude Code SDK (TypeScript)". Its dependency is `@anthropic-ai/claude-agent-sdk` 0.2.44 [verified: `npm view`]. So the adapter wraps the Agent SDK, not the CLI process. Repo: https://github.com/zed-industries/claude-code-acp [docs S14].
- Server or API mode: `claude mcp serve` starts Claude Code as an MCP server [verified: `claude mcp --help`]. `claude agents --json` prints session state for scripts [verified; see §10]. No HTTP API found.
- Agent SDK: `@anthropic-ai/claude-agent-sdk` on npm (name confirmed as the adapter's dependency). Python and TypeScript SDK packages are documented [docs S2]; Python package name not checked.

## 4. Conversation identity

- **Caller-chosen id:** yes. `--session-id <uuid>` ("must be a valid UUID") [verified: `claude --help`]. Verified end to end: the `SessionStart` hook's stdin `session_id` equalled the UUID passed on the command line [verified: print-mode probe].
- **Learning the id:** if the caller passed it, it is already known. Otherwise: `session_id` in the `-p --output-format json` result [docs S2]; `session_id` in every hook's stdin [verified for SessionStart; docs S1]; `sessionId` in `claude agents --json` [verified key].
- **Resume by id:** `claude --resume <session-id>` works from any directory. From another project, the id is found across projects, but only when exactly one other project holds a transcript for it; a duplicate makes the lookup fail [docs S3]. `claude -p --resume <id> "prompt"` also works [docs S2]. `--resume <path/to/transcript.jsonl>` works too [docs S3].
- **Resume does not restore per-process flags.** A resumed session does not restore `--mcp-config`, `--settings`, `--plugin-dir`, `--fallback-model`, or `--add-dir`; they must be passed again [docs S3]. The permission mode is restored [docs S3].
- **System prompt snapshot:** `--system-prompt-snapshot on|off`, default `on`. The first request's rendered system prompt is recorded and reused on every later request and on resume, even if the launch passes different text, until the conversation is compacted [verified: `claude --help`].
- **Storage:** `$CLAUDE_CONFIG_DIR/projects/<project>/<session-id>.jsonl` (default config dir `~/.claude`). `<project>` is the working directory path with non-alphanumeric characters replaced by `-` [verified: `~/.claude/projects/` listing shows `-private-tmp-…/<uuid>.jsonl`; the probe's transcript path was under `$CLAUDE_CONFIG_DIR/projects/`; docs S3]. The line format is internal and changes between versions [docs S3]. Transcripts are written asynchronously and can lag the live conversation [docs S1].
- **Several instances in one directory:** each session has its own transcript file, so ids do not collide. `claude --continue` reopens the most recent session in the cwd [docs S3], which is ambiguous with several instances. Resume by id. `-p` sessions are left out of the session picker and `--continue` [docs S3], but `--resume <id>` still finds them.

## 5. Hooks and events

- **Events:** 33 hook events [docs S1]. Session: `SessionStart` (matcher `startup|resume|clear|compact|fork`), `SessionEnd`, `Setup`. Per turn: `UserPromptSubmit`, `Stop`, `StopFailure`. Tools: `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `PermissionDenied`, `PostToolBatch`. Subagents and tasks: `SubagentStart`, `SubagentStop`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`. Display: `Notification`, `MessageDisplay`. Others: `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `DirectoryAdded`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `PreCompact`, `PostCompact`, `PreModelSwitch`, `PostModelSwitch`, `Elicitation`, `ElicitationResult` [docs S1].
- **Config format** [docs S1]: `settings.json` → `"hooks"` → event name → array of `{ "matcher": "...", "hooks": [ { "type": "command", "command": "...", "timeout": 30 } ] }`. Handler types: `command`, `http`, `mcp_tool`, `prompt`, `agent`. Command default timeout 600 s.
- **Config locations** [docs S1]: `~/.claude/settings.json`, `.claude/settings.json`, `.claude/settings.local.json`, managed policy settings, plugin `hooks/hooks.json`, skill/subagent frontmatter.
- **Per-process injection: yes, via `--settings`.** [verified] A `SessionStart` command hook passed only as `--settings '{"hooks":{...}}'` ran in `claude -p` with no settings file edits. Hook stdin keys: `cwd`, `hook_event_name`, `session_id`, `source`, `transcript_path` [verified]. `cwd` is the resolved path (`/private/var/…`), not the symlinked one.
- `--settings` sits above user, project, and local settings and below managed settings [docs S5]. A `disableAllHooks` key disables all hooks [docs S1]. Managed hooks cannot be overridden [docs S5].
- **Exit codes and JSON** [docs S1]: exit 0 means success, and stdout is parsed as JSON if it is a JSON object. Exit 2 is a blocking error: stderr is fed back to Claude, and it blocks `PreToolUse`, `UserPromptSubmit`, `Stop`, `SubagentStop`. Any other code is non-blocking. JSON fields include `decision: "block"` with `reason`, `continue`, `systemMessage`, and `hookSpecificOutput.additionalContext` (cap 10,000 chars).
- **Turn end:** a `Stop` hook can prevent the stop with exit 2 or `decision: "block"`, which continues the conversation with the reason. `stop_hook_active` is `true` when already continuing from a stop hook. There is an 8-consecutive-continuation cap [docs S1]. `Stop` input includes `last_assistant_message` [docs S1].
- **Permission signals:** `PermissionRequest` fires when a tool needs a decision, immediately [docs S1]. `Notification` with `notification_type: "permission_prompt"` fires about 6 s after the prompt appears, deferred by typing [docs S1]. `idle_prompt` fires about 60 s after a finished response if no one typed and no background agent is running [docs S1].

## 6. Plugins and extensions

- Per-process: yes. `--plugin-dir <path>` loads a plugin directory or `.zip` for this session only. It is repeatable, and a folder of plugins loads each child. `--plugin-url <url>` fetches a zip for the session [verified: `claude --help`].
- Layout [docs S11]: optional `.claude-plugin/plugin.json` (`name` is the only required key); `skills/`, `commands/`, `agents/`, `hooks/hooks.json`, `.mcp.json` at the plugin root. The manifest can add more paths. Plugins can contribute hooks, MCP servers, commands, agents, and skills.
- Global, not per-process: `claude plugin install|enable|disable` change installed state [verified: `claude plugin --help`]. `claude plugin init|new <name>` scaffolds into `~/.claude/skills/<name>/` and auto-loads it next session [verified: help text].
- `--agents <json-or-file>` defines subagents per process [verified: help].

## 7. MCP

- Transports [docs S10]: stdio; HTTP (`claude mcp add --transport http`; JSON `type: "http"`, with `streamable-http` accepted as an alias); SSE (deprecated, `--transport sse`). `claude mcp add-json` help also lists WebSocket [verified: help].
- Per-process MCP config: yes. `--mcp-config <configs...>` takes JSON files or inline JSON strings. `--strict-mcp-config` ignores all other MCP config [verified: help].
- The current Hypercube already uses this. Live sessions were started as `claude --mcp-config {"mcpServers":{"hypercube":{"args":["mcp"],"command":"<hypercube binary>","env":{"HYPERCUBE_AGENT_ID":"…","HYPERCUBE_STATE":"…"}}}}` [verified: `ps`].
- Invalid `--mcp-config` entries are skipped, and the skip is reported in stream-json `init` as `mcp_server_errors` [docs S2]. Interactive sessions print a warning to stderr [docs S2].
- In `-p` mode, project `.mcp.json` servers connect without an approval prompt unless `--bare` is set [docs S2].

## 8. Instructions and customization

- System prompt: `--system-prompt <prompt>` replaces the default; `--append-system-prompt <prompt>` appends [verified: help]. The help text also names `--system-prompt-file` and `--append-system-prompt-file` [verified: `--bare` help text; docs S2 table].
- `--system-prompt-snapshot` is described in §4.
- Subagents and agents: `--agent <name>`, `--agents <json-or-file>` [verified]. Subagents can also come from `.claude/agents/` [docs S2].
- Model and effort: `--model <alias or full name>` (aliases `fable`, `opus`, `sonnet` per help), `--fallback-model <list>` for automatic fallback, `--effort low|medium|high|xhigh|max` [verified: help].
- Permissions: `--permission-mode acceptEdits|auto|bypassPermissions|manual|dontAsk|plan`, `--allowedTools`, `--disallowedTools`, `--tools` [verified: help]. In print mode, `--permission-prompts host|none`; `none` denies anything that would prompt [verified: help; docs S2].
- `--bare` [verified: help]: skips hooks, LSP, plugin sync, auto-memory, CLAUDE.md discovery, and keychain reads. It sets `CLAUDE_CODE_SIMPLE=1`. Anthropic auth becomes strictly `ANTHROPIC_API_KEY` or `apiKeyHelper` through `--settings`. OAuth and keychain are never read.
- `--setting-sources user,project,local` restricts which settings files load [verified: help]. `--safe-mode` disables customizations [verified: help]. `--restricted` removes code-running tools and ignores user/project/local settings [verified: help].
- Third-party providers (Bedrock, Vertex, Foundry) use their own credentials [docs S2]. Env var names not checked.

## 9. Getting messages into a running interactive session

- No documented API for a foreground interactive session. Docs say an interactive session started from a terminal is tied to that terminal [docs S6].
- Typing into the PTY is the only generic path. Bracketed paste mode is on at startup [verified], but submitting a message through the PTY was not tested.
- Turn-end feedback: a `Stop` hook with `decision: "block"` or exit 2 continues the turn with that text. It only fires when the agent is ending a turn; it cannot wake an idle session [docs S1].
- `UserPromptSubmit` `additionalContext` adds context to a prompt the user submits. It does not inject a new prompt [docs S1].
- Background sessions only: `claude --resume <id> "message"` sends the message as the session's next turn, then attaches the terminal. `claude -p --resume <id> "message"` typed at a terminal does the same, and `-p` does not keep that run non-interactive [docs S3, S6]. Replies sent while a session is working join its message queue [docs S6]. These sessions are owned by Claude's supervisor (§12).
- Unknown: whether `--remote-control [name]` gives an external message channel. The flag exists [verified: help]; its semantics were not checked.

## 10. Status detection from outside

- **`claude agents --json --all`** [verified; keys seen: `cwd, id, kind, name, pid, sessionId, startedAt, state, status`]. Live output showed interactive sessions with `status` `busy` (6) or `idle` (5), and background sessions with `state` `done` or `stopped`. No `waitingFor` key appeared, because no session was blocked during the check.
- Documented values [docs S6]: `state` is `working | blocked | done | failed | stopped`. `status` is `busy` or idle. `waitingFor` names the open prompt when the process is alive and waiting. Whether an interactive session reports `blocked` in this JSON: unknown, not observed.
- **Hook signals** [docs S1]: `UserPromptSubmit` (turn starts), `PermissionRequest` (permission needed), `Notification` `permission_prompt` (waiting ~6 s) and `idle_prompt` (idle ~60 s), `Stop` (turn ends), `SessionEnd` (exits). Per-process via `--settings` [verified for SessionStart].
- **OSC 7501 Program Status Protocol** [docs S7]: Claude reports working, waiting, or done to a terminal that implements OSC 7501. Requires Claude Code v2.1.295 or later. Not used inside tmux. Not used for background sessions. Turn it off with `CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1`. The escape-sequence format is not in the page. Unknown: I grepped the local binary for `7501` and found nothing, so I could not confirm the bytes.
- `statusLine` gets session JSON on stdin, including `session_id`, `transcript_path`, `cost`, `context_window`, `rate_limits` [docs S8]. Per-process setup via `--settings`: unknown, not tested.

## 11. Usage and cost

- `-p --output-format json`: `total_cost_usd` and per-model cost breakdown [docs S2]. Client-side estimate. Differs from the bill [docs S2].
- `--max-budget-usd <amount>`: print mode only [verified: help]. When the cap is reached, Claude Code stops the remaining background work instead of waiting for it [docs S2].
- Statusline JSON: `cost.total_cost_usd`, `cost.total_duration_ms`, `cost.total_api_duration_ms`, `context_window.used_percentage`, `rate_limits.five_hour.used_percentage`, `rate_limits.seven_day.used_percentage` [docs S8].
- OpenTelemetry: `CLAUDE_CODE_ENABLE_TELEMETRY=1`, `OTEL_METRICS_EXPORTER=otlp|console|prometheus|none`, `OTEL_LOGS_EXPORTER=otlp|console|none` [docs S9]. These are env vars, so they can be set per process. The docs say a repo's `.claude/settings.json` cannot set the OTEL exporter variables [docs S9].
- `/usage` and `/cost` exist as commands [docs S6; not run].

## 12. Gotchas for many instances

- **`--bg` is not a quiet flag.** `claude --bg --help` does not print help. It started a background session (`1d2761dc`, `state: done`) [verified]. Keep `--help` out of argv you build with `--bg`.
- **The background supervisor is a separate, persistent daemon.** The first `--bg` or `claude agents` starts it [docs S6]. Its log is `~/.claude/daemon.log`. It logged `idle 5s with no clients — exiting` once its only session had finished [verified: daemon.log]. A finished session unattached for about an hour is stopped [docs S6]. If the supervisor is killed, sessions stop unless a new supervisor connects within about a minute [docs S6].
- **Process name:** one spawned claude process showed up in `ps` as `(2.1.312)`, not `claude` [verified once]. Match by PID and process group.
- **Config isolation:** with `CLAUDE_CONFIG_DIR` set, a run left the global `~/.claude.json` mtime unchanged [verified]. Without it, settings, sessions, plugins, and credentials live in shared `~/.claude` [docs S4]. Behavior of concurrent writers to `~/.claude.json` without the variable: unknown.
- **Credentials:** the supervisor and its sessions use the same stored credentials as interactive sessions [docs S6]. `--bare` reads only `ANTHROPIC_API_KEY` or `apiKeyHelper` [verified: help].
- **Zsh wrapper on this machine:** in interactive zsh, `claude` is a shell function. It sources a wrapper only in repos whose `.claude/settings.local.json` sets a specific OTEL endpoint, and otherwise runs `command claude` [verified: `whence`/function body]. Launch by absolute path (`~/.local/bin/claude`) to avoid it.
- **Resume drops per-process flags** (§4). Re-pass them on every resume.
- **Transcript lag:** do not read the transcript to get the last assistant message. Use `last_assistant_message` on Stop [docs S1].
- **Fullscreen depends on the user's `tui` setting** (§2). Set `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN=1` or `CLAUDE_CODE_NO_FLICKER=1` per process to pin the mode.
- `--session-id` requires a UUID [verified: help].

## 13. Fit for Hypercube

- **Launch:** per agent, `claude --session-id <uuid> --settings '<json with hooks>' --mcp-config '<json with hypercube MCP server and HYPERCUBE_AGENT_ID>' --append-system-prompt '…'`, with the absolute binary path. Everything is per process; no user config file has to change. Verified for `--session-id`, `--settings` hooks, and `--mcp-config`, which the current Hypercube already uses.
- **Conversation tracking:** the caller picks the UUID, so no discovery step is needed. Resume with `--resume <uuid>` and re-pass every per-process flag, because resume drops them.
- **Status:** the cleanest option is per-process hooks (`UserPromptSubmit`, `PermissionRequest`, `Notification`, `Stop`, `SessionEnd`) posting to Hypercube's registry over loopback. `claude agents --json` gives busy/idle for interactive sessions. Gap: no verified "blocked/waiting" signal for interactive sessions, and the OSC 7501 format is unknown.
- **Message delivery:** the gap. No documented way to inject a message into an idle interactive session. Options: PTY typing (submit not verified), a `Stop`-hook feedback loop (works only at turn end), or background sessions with `claude --resume <id> "msg"`. Background sessions move the process under Claude's supervisor, which breaks the ownership rule that Hypercube owns its PTY's processes. Avoid `--bg` for owned agents.
- **Missing:** a documented signal for "waiting for user" in interactive mode, the OSC 7501 byte format, an xterm.js check, and a message-injection API for foreground sessions.

## Unknown (not checked)

- xterm.js rendering and input behavior.
- Whether typed input through the PTY is accepted and submitted reliably.
- Blocked/waiting state for interactive sessions in `claude agents --json`.
- OSC 7501 byte format.
- Provider environment variable names for Bedrock, Vertex, and Foundry.
- Per-process `statusLine` setup.
- Whether `--remote-control` offers an external message channel.
- Concurrent writes to `~/.claude.json` without `CLAUDE_CONFIG_DIR`.

## Sources

- S1 https://code.claude.com/docs/en/hooks (raw: https://code.claude.com/docs/en/hooks.md)
- S2 https://code.claude.com/docs/en/headless
- S3 https://code.claude.com/docs/en/sessions
- S4 https://code.claude.com/docs/en/env-vars
- S5 https://code.claude.com/docs/en/settings
- S6 https://code.claude.com/docs/en/agent-view
- S7 https://code.claude.com/docs/en/terminal-config
- S8 https://code.claude.com/docs/en/statusline
- S9 https://code.claude.com/docs/en/monitoring-usage
- S10 https://code.claude.com/docs/en/mcp
- S11 https://code.claude.com/docs/en/plugins-reference
- S12 https://code.claude.com/docs/en/fullscreen
- S13 https://code.claude.com/docs/en/setup
- S14 https://github.com/zed-industries/claude-code-acp (npm: `@zed-industries/claude-code-acp`)
- Local: `claude --help`, `claude --version`, `claude mcp --help`, `claude agents --help`, `claude plugin --help`, `claude agents --json --all`, `npm view`, `gh api`, the PTY probes (`/tmp/hc-research/pty_probe.py`, `resize_probe.py`), `~/.claude/projects/` listing, `~/.claude/daemon.log`.
