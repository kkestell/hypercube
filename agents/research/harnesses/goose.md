# Goose (Block)

Not installed here, and nothing was run. Every fact is **[docs]**: the official docs or the Rust source at `block/goose` `main` (read 2026-10-29), linked on each fact. Source at `main` may be ahead of the v1.57.0 tag. Nothing is marked [verified].

## 1. Overview

- Open-source general-purpose agent with a CLI (`goose`), a desktop app, and an ACP server. [docs](https://github.com/block/goose)
- Vendor: Block. Install docs now point at `aaif-goose/goose` releases. [docs](https://github.com/block/goose/blob/main/documentation/docs/getting-started/installation.md)
- License: Apache-2.0. [docs](https://github.com/block/goose)
- Current version: v1.57.0, published 2026-10-28. [docs](https://github.com/block/goose/releases/tag/v1.57.0)
- A 2.0 beta is in progress. It moves the interactive UI to a TypeScript TUI on ACP, and "Remove goosed and the old Rust CLI" is listed as Planned. The CLI surface below may change. [docs](https://github.com/block/goose/blob/main/documentation/blog/2026-04-08-goose-acp-and-new-tui/index.md)
- Install: `brew install block-goose-cli`, or `curl -fsSL https://github.com/aaif-goose/goose/releases/download/stable/download_cli.sh | CONFIGURE=false bash` (omit `CONFIGURE=false` to run setup). [docs](https://github.com/block/goose/blob/main/documentation/docs/getting-started/installation.md)
- Adoption: 55,912 GitHub stars; created 2024-08-23; last push 2026-10-29. [docs](https://github.com/block/goose)
- Language: Rust workspace under `crates/`. [docs](https://github.com/block/goose/tree/main/crates)

## 2. Interactive mode in a PTY

- `goose session` is a line REPL, not a full-screen TUI. Input is `rustyline`; output streams as text. I found no alternate-screen code in `session/mod.rs` or `session/input.rs`. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/Cargo.toml), [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- Tool approvals and questions are terminal prompts (`prompt_tool_confirmation`, elicitation). [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- When stdout is a TTY it writes two things into the byte stream: the OSC 7501 status sequence (section 10), and a BEL when an interactive turn ends or approval is needed, if `GOOSE_CLI_BELL=true` (default false). [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md), [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/output.rs)
- On exit it prints `session closed · <session id>`. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- xterm.js compatibility: unknown, not tested. Things to test: whether the OSC 7501 sequence passes through the server's headless xterm without side effects, and rustyline's cursor and paste handling.
- Resize behavior: unknown, not tested.

## 3. Non-interactive and server modes

- `goose run` flags: `-t/--text <TEXT>`, `-i/--instructions <FILE>` (`-` reads stdin), `--recipe`, `--params KEY=VALUE`, `--system <TEXT>`, `-s/--interactive` (keep going interactively after the first input), `-n/--name`, `-r/--resume`, `--no-session`, `--max-turns` (default 1000), `--provider`, `--model`, `--output-format text|json|stream-json` (default text). [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md), [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- `--output-format stream-json` writes one JSON object per line, tagged by `type` (`#[serde(tag = "type", rename_all = "snake_case")]`). Variants: `message` (`message`), `notification` (`extension_id` plus `log` or `progress` data), `error` (`error`), and `complete` (`total_tokens`, `input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_write_input_tokens`, `cost_usd`; absent fields are omitted). [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- `--output-format json`: a single final result. Its shape was not traced: unknown.
- Approval in headless runs: in `approve` or `smart_approve` mode, a tool confirmation makes the run fail with "Tool approval required in non-interactive mode". In `auto` (the default), tool calls are allowed without asking. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- `goose acp`: ACP agent on stdio, native (not an adapter). [docs](https://github.com/block/goose/blob/main/documentation/docs/gdk/acp/index.md)
- `goose serve`: ACP over HTTP and WebSocket, behind the `acp-http` cargo feature in source (whether the release build enables it: unknown). Default `127.0.0.1:3284`. Flags: `--host`, `--port`, `--tls`, `--tls-cert-path`, `--tls-key-path`, `--with-builtin`, `--allowed-origin` (replaces the default loopback origins), `--dangerously-unauthenticated`. Clients authenticate with the `GOOSE_SERVER__SECRET_KEY` secret sent as an `X-Secret-Key` header. [docs](https://github.com/block/goose/blob/main/documentation/docs/gdk/acp/index.md), [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- ACP session model: `session/new` returns a server-assigned id. A `session/load` handler exists and replays history. [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/new_session.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/load_session.rs)
- Custom ACP requests (type names) include `SteerSessionRequest`, `SetSessionSystemPromptRequest`, `AddSessionExtensionRequest`, `RemoveSessionExtensionRequest`, `GetToolsRequest`, `SetToolPermissionsRequest`, `UpdateWorkingDirRequest`. [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/custom_dispatch.rs)
- Limitation for goose wrapping another ACP agent (ACP providers): `goose session resume` and `fork` are not supported. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/acp-providers.md)

## 4. Conversation identity

- Session id format: `YYYYMMDD_<COUNT>`, e.g. `20260213_9`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/sessions/session-management.md)
- Start with a caller-chosen id: **not supported**. `--session-id` requires `--resume`; without it the CLI fails with "Cannot use --session-id without --resume". ACP `session/new` assigns the id. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/new_session.rs)
- Start with a caller-chosen name: yes, `-n/--name` (stored as `user_provided_name`). [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- Learn the id after start: (a) the `session_id` field on every hook payload (section 5); (b) `goose session list --format json` (filters `-w <dir>`, `-l <n>`); (c) the `session closed · <id>` line on exit. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md)
- Resume: `goose session --resume --session-id <id>`; `goose session --resume --name <name>` (matches name or id, first match wins); `goose session --resume` alone resumes the most recent user session **across all directories**, with no working-directory filter. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- Fork: `--resume --fork` copies history into a new session. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/sessions/session-management.md)
- Storage: SQLite `sessions.db` under `<data dir>/sessions/`. The docs say `~/.local/share/goose/sessions/sessions.db`. Source uses the platform data directory for `Block/goose`, or `$GOOSE_PATH_ROOT/data/` when `GOOSE_PATH_ROOT` is set. Confirm the real path with `goose info -v` before relying on it. [docs](https://github.com/block/goose/blob/main/crates/goose/src/session/session_manager.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/config/paths.rs)
- Several instances in one directory: each `goose session` without `--resume` creates a new session record. Concurrent writes to the shared SQLite file: unknown (locking and WAL not checked). Whether names must be unique: unknown.

## 5. Hooks and events

- Events: `SessionStart`, `SessionEnd`, `Stop`, `UserPromptSubmit`, `PreToolUse`, `PreToolUseResult`, `PostToolUse`, `PostToolUseFailure`, `BeforeReadFile`, `AfterFileEdit`, `BeforeShellExecution`, `AfterShellExecution`. `SubagentStart` and `SubagentStop` are named in the docs but are not emitted. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Configuration: `hooks/hooks.json` inside a plugin directory: `{"hooks": {"<Event>": [{"matcher": "<regex>", "hooks": [{"type": "command", "command": "...", "timeout": 30}]}]}}`. `matcher` is an unanchored regex and is optional. Commands run via `sh -c`; `${PLUGIN_ROOT}` expands to the plugin directory. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Stdin payload (JSON): `event`, `session_id`, `matcher_context`. Tool events add `tool_name`, `tool_input`, `tool_call_id`, `working_dir`. `UserPromptSubmit` adds `message`. `Stop` adds `last_assistant_message`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Decisions: only `PreToolUse` and `Stop` can block. Block with exit code 2 (reason on stderr) or with stdout `{"decision":"block","reason":"..."}`. Other events cannot stop anything, and the docs define no output field that feeds text back to the model from a non-blocking hook. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Stop block mechanics: a blocked Stop keeps the turn going, and the reason is added as a user-role message that the agent sees and the user does not: "Stop hook `<plugin>` blocked ending this turn: <reason> Address this policy hook denial before trying to stop again." [docs](https://github.com/block/goose/blob/main/crates/goose/src/agents/state_machine/ops_stop_hook.rs)
- Consecutive Stop blocks are capped; `GOOSE_STOP_HOOK_BLOCK_CAP` raises the cap. Its default: unknown (not located in the files read). [docs](https://github.com/block/goose/blob/main/crates/goose/src/agents/state_machine/ops_stop_hook.rs)
- `PreToolUse` with `on_failure: "block"` denies the call if the hook fails; default `allow`. `Stop` is always fail-open. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Per-process injection without editing user config: partial. No CLI flag or env var names a hook. Discovery roots are the user plugin dir (`~/.agents/plugins/`, or `$GOOSE_PATH_ROOT/.agents/plugins/` when `GOOSE_PATH_ROOT` is set) and the project's `<project>/.agents/plugins/`. Setting `GOOSE_PATH_ROOT` moves config, data, state, and the user plugin dir together, so a per-process hook set means a private root (side effects in section 12). [docs](https://github.com/block/goose/blob/main/crates/goose/src/config/paths.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/plugins/discovery.rs)
- Plugins can be disabled with `disabledPlugins` in `~/.config/goose/settings.json` or `<project>/.config/goose/settings.json`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)

## 6. Plugins/extensions

- Format: Open Plugins spec. A directory with `plugin.json` (`name`, `version`, `description`), `hooks/hooks.json`, optional `scripts/`, and optional `.mcp.json` (plugin-supplied MCP servers). [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md), [docs](https://github.com/block/goose/blob/main/crates/goose/src/plugins/mcp_servers.rs)
- Install from git: `goose plugin install <git-url>`, `goose plugin update`. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- Loading: at startup from the discovery roots in section 5. Load per process only through `GOOSE_PATH_ROOT`.
- In the docs I read, the only in-process extension points are hooks and MCP (section 7). I did not find a code plugin API.

## 7. MCP

- Client: yes. Extension types in `config.yaml`: `builtin`, `platform`, `stdio`, `streamable_http`. SSE is not supported. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md)
- Per-process flags on `session` and `run`: `--with-extension "ENV1=val1 ENV2=val2 command args..."` (repeatable), `--with-streamable-http-extension <url> [timeout=N]` (repeatable), `--with-builtin <names>`, `--no-profile` (drop default extensions), `--container <id>` (run extensions in Docker). The env-assignment splitting rules were not traced. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md), [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/cli.rs)
- ACP: `session/new` accepts goose extensions in `_meta` (`enabledExtensions`). Running sessions can add or remove MCP servers with `AddSessionExtensionRequest` / `RemoveSessionExtensionRequest`. [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/new_session.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/custom_dispatch.rs)
- Tool naming: `{extension}__{tool}` (e.g. `todo__todo_write`). `developer`, `analyze`, `summon`, and `code_execution` are exposed without the prefix. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)

## 8. Instructions and customization

- System prompt per process: `--system <TEXT>` (documented on `run`; whether `session` takes it: unknown). Per-session override over ACP: `SetSessionSystemPromptRequest`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md), [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/custom_dispatch.rs)
- Recipes: YAML with instructions, prompt, parameters, sub-recipes. `run --recipe`, `--params`, `--sub-recipe`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md)
- Subagents: `summon` extension, `GOOSE_MAX_CONCURRENT_SUBAGENTS` (default 5), `GOOSE_SUBAGENT_MAX_TURNS`, `GOOSE_SUBAGENT_PROVIDER` / `GOOSE_SUBAGENT_MODEL`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md)
- Models: `GOOSE_PROVIDER` and `GOOSE_MODEL` override `config.yaml` for that process. Credentials per process: `GOOSE_PROVIDER__TYPE`, `GOOSE_PROVIDER__HOST`, `GOOSE_PROVIDER__API_KEY`. `run --provider` / `--model`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md), [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md)
- Permission modes: `GOOSE_MODE` = `auto` (default), `approve`, `smart_approve`, `chat`. Changeable mid-session with `/mode <name>`. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/managing-tools/goose-permissions.md)
- Project instruction files (AGENTS.md or similar): unknown, not checked.

## 9. Getting messages INTO a running interactive session from outside

- The interactive `goose session` process has no control socket or API that I found. Its input is the PTY.
- Turn-end injection: a `Stop` hook block delivers its reason as a user message and continues the turn (section 5). It is the only text-injection path found in the hooks. The text arrives inside fixed wording ("Stop hook `<plugin>` blocked ending this turn ... Address this policy hook denial before trying to stop again").
- Mid-turn steering: the core has a steer queue ("Adds queued user guidance when the agent is between model and tool turns"). It is reached over ACP through `SteerSessionRequest`, so it exists for `goose acp` / `goose serve` sessions. No external entry point for the interactive CLI process was found. [docs](https://github.com/block/goose/blob/main/crates/goose/src/agents/state_machine/ops_steer.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/acp/server/custom_dispatch.rs)
- Waking an idle session from outside: no API found. A Stop hook only fires at turn end. The only route found for an idle `goose session` is typing into the PTY.
- `UserPromptSubmit`, `PostToolUse`, and the other non-blocking events cannot inject text. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)

## 10. Status detection from outside

- **OSC 7501 status sequence.** Written only in text output mode, when stdout is a TTY, unless `GOOSE_CLI_PROGRAM_STATUS` is false. Format: `ESC ] 7501 ; state=<state>[:kind=<permission|question>]:app=goose[:msg=<base64, capped at 2048 bytes>] ESC \`. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/program_status.rs)
- States and where they fire (in `session/mod.rs`):
  - `working` at the start of each reply.
  - `blocked:kind=permission` before a tool-approval dialog, then `working` again after it.
  - `blocked:kind=question` for elicitation.
  - `done` after an interactive turn completes. `idle` only if the turn was cancelled.
  - `error` on failure. `clear` on normal session close.
  [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- **"Waiting for input" is reported as `done`.** `report_idle()` runs before each input prompt, but it does not overwrite `done` or `error`. `idle` appears at the first prompt of a fresh session, and after a cancel. Hypercube should treat `done` as idle, and `idle` as cancelled. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/program_status.rs)
- **`GOOSE_STATUS_HOOK`** (source only, not in the docs). When set, goose runs `sh -c "<value> <status>"` in the background with output discarded: `waiting` before each input prompt, `thinking` when a turn starts. Read through `Config::get_param`, which checks the environment first. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/output.rs), [docs](https://github.com/block/goose/blob/main/crates/goose/src/config/base.rs)
- Hooks: `UserPromptSubmit` (turn starts) and `Stop` (turn ends) bracket each turn. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md)
- Permission requests: no hook event. The `blocked` state above is the only signal found.
- Headless: the `complete` stream event marks the end of a run. Process exit also ends it.

## 11. Usage and cost reporting

- stream-json `complete` event: `total_tokens`, `input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_write_input_tokens`, `cost_usd` (optional). [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- Interactive: `GOOSE_CLI_SHOW_COST=true` shows estimated cost (default false). [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md)
- Sessions store accumulated usage and cost. `SessionManager` exposes `get_session_usage_totals` and `get_insights`. [docs](https://github.com/block/goose/blob/main/crates/goose/src/session/session_manager.rs)
- Field contents of `session list --format json` and `session export`: unknown, not checked. How cost is priced: unknown.

## 12. Gotchas for running many instances side by side

- `GOOSE_PATH_ROOT` (absolute path) moves config, data (`sessions.db`), state, and the user plugin directory into one root. A process started with it does not see `~/.config/goose/config.yaml`, and credentials stored in the keyring or the config directory may be missing. Whether the keyring still applies: unknown. Test before relying on it. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md), [docs](https://github.com/block/goose/blob/main/crates/goose/src/config/paths.rs)
- `--resume` with no id resumes the most recent session anywhere, not the one for this directory. Always pass `--session-id` or `--name`.
- `GOOSE_DISABLE_SESSION_NAMING=1` stops the background model call that names each session. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md)
- `GOOSE_DISABLE_KEYRING` avoids keychain access. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/environment-variables.md)
- `goose serve` defaults to port 3284; set `--port` per instance. It requires `GOOSE_SERVER__SECRET_KEY` unless `--dangerously-unauthenticated` is passed. [docs](https://github.com/block/goose/blob/main/documentation/docs/gdk/acp/index.md)
- `--max-turns` defaults to 1000. [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/goose-cli-commands.md)
- Non-interactive `approve` / `smart_approve` runs fail; use `auto` for headless work. [docs](https://github.com/block/goose/blob/main/crates/goose-cli/src/session/mod.rs)
- Stop hooks are fail-open and capped (section 5). Cap default: unknown.
- Shared `sessions.db` across processes: locking behavior unknown.
- Telemetry defaults to off (`GOOSE_TELEMETRY_ENABLED`). [docs](https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md)
- The 2.0 transition may change the interactive CLI, OSC output, and flags. Pin a version.

## 13. Fit for Hypercube

- **Launch:** `goose session --name <label>` with `--with-extension "HYPERCUBE_AGENT_ID=… HYPERCUBE_STATE=… '<exe>' mcp"` for the MCP server. The first prompt either goes to the PTY after startup, or through `goose run -t "Check your Hypercube messages." -s`, which is untested as a PTY process. Hooks need a private `GOOSE_PATH_ROOT` per agent, which also isolates credentials and sessions (section 12).
- **Conversation tracking:** no caller-chosen id. Read `session_id` from the first `SessionStart` hook payload and store it. Resume with `--resume --session-id <id>`. Use `--name` only as a label.
- **Status:** the OSC 7501 stream maps directly to working, blocked (permission or question), and done. `Stop` and `UserPromptSubmit` hooks are the backup. `GOOSE_STATUS_HOOK` is undocumented, so use it only as a fallback.
- **Message delivery:** busy agent on `goose acp` / `goose serve`: `SteerSessionRequest`. Busy agent in the PTY: queue until the `Stop` hook, which delivers the text inside fixed wording. Idle agent in the PTY: type into the PTY. There is no equivalent to `hypercube wait` exiting with status 2 that wakes an idle interactive session without typing.
- **Gaps:** no caller-chosen ids; no non-blocking context injection; no permission-request hook (OSC only); per-process hook isolation only through `GOOSE_PATH_ROOT`; the interactive UI is untested under xterm.js and may be replaced by the 2.0 TUI.
