# Goose

## 1. Overview

- **[verified]** Research 2026-10-30; `goose` not on PATH. No install, config mutation, runtime session or model call.
- **[docs]** Block's Apache-2.0 Rust agent; CLI and desktop share core. Latest release API returned `v1.57.0` (2026-10-28). Sources below are current main, not guaranteed release-identical. Install via official release binaries/Homebrew/install script; installation not performed. https://github.com/block/goose https://api.github.com/repos/block/goose/releases/latest
- Exact release cadence, active CLI users and resident memory: unknown.

## 2. Interactive terminal interface

- **[docs]** `goose session` starts interactive chat; `goose run --interactive` continues interactively after initial instructions. Rendering variables: `GOOSE_CLI_THEME=light|dark|ansi`, `GOOSE_CLI_LIGHT_THEME`, `GOOSE_CLI_DARK_THEME`, `GOOSE_CLI_NEWLINE_KEY` (default Ctrl+J), `GOOSE_PROMPT_EDITOR`. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** `GOOSE_CLI_BELL=true` rings on completion/approval, default false. Thinking `GOOSE_CLI_SHOW_THINKING` (any set value), random messages `GOOSE_RANDOM_THINKING_MESSAGES`, costs `GOOSE_CLI_SHOW_COST`, truncation `GOOSE_MAX_CODE_BLOCK_LINES=50`, `GOOSE_TRUNCATED_SHOW_LINES=20`, `GOOSE_NO_CODE_TRUNCATION`, `GOOSE_SHOW_FULL_OUTPUT`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- Fullscreen/alternate screen/mouse/resize/title/OSC 9/9;4/7501 exact behavior: unknown.

## 3. Launch and configuration

- **[docs]** Strong process state isolation knob: `GOOSE_PATH_ROOT=ABSOLUTE_PATH` relocates config/data/state into `config/`, `data/`, `state/`. Environment overrides config files; `--provider`/`--model` override their environment equivalents. Default user config examples use `~/.config/goose/config.yaml`/`settings.json`; default macOS application data root `~/Library/Application Support/Block/goose/`, Linux `~/.local/share/goose/`, Windows `%APPDATA%\Block\goose\`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** Shared session/run flags: `--debug`, `--max-tool-repetitions NUMBER`, `--max-turns NUMBER` (default 1000), `--container CONTAINER_ID`, repeat `--with-extension '[name:]ENV=val command args'`, repeat `--with-streamable-http-extension 'URL timeout=100'`, `--with-builtin developer,github`, `--no-profile` (only CLI extensions), `--provider`, `--model`. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** Run input: mutually exclusive `--instructions/-i FILE` (`-` stdin), `--text/-t TEXT`, `--recipe NAME_OR_PATH`; `--system TEXT` (conflicts recipe), repeated `--params KEY=VALUE`, repeated `--sub-recipe`, `--explain`, `--render-recipe`. Execution: `--interactive/-s`, `--no-session`, `--resume/-r`, `--stats`, `--quiet/-q`, `--output-format text|json|stream-json`. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** Process env beyond model settings: `GOOSE_MODE`, `GOOSE_SHELL`, `GOOSE_SEARCH_PATHS` JSON array, `GOOSE_DEBUG`, `GOOSE_CLI_MIN_PRIORITY`, `GOOSE_MAX_TOOL_RESPONSE_SIZE` (200000 chars), `CONTEXT_FILE_NAMES` JSON array (defaults `.goosehints`,`AGENTS.md`), `GOOSE_MOIM_MESSAGE_TEXT` and `GOOSE_MOIM_MESSAGE_FILE` (persistent turn instructions; file max 64 KiB), `GOOSE_RECIPE_PATH`, `GOOSE_RECIPE_GITHUB_REPO`, recipe retry/on-failure timeout variables. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** Proxy `HTTP_PROXY`,`HTTPS_PROXY`,`NO_PROXY`; privacy/security `GOOSE_TELEMETRY_ENABLED`, `GOOSE_ALLOWLIST`, `GOOSE_DISABLE_KEYRING`, `SECURITY_PROMPT_ENABLED`,`SECURITY_PROMPT_THRESHOLD`,`SECURITY_PROMPT_CLASSIFIER_ENABLED`,`SECURITY_PROMPT_CLASSIFIER_ENDPOINT`,`SECURITY_PROMPT_CLASSIFIER_TOKEN`. OTel `OTEL_EXPORTER_OTLP_ENDPOINT`, per-signal endpoint/exporter variables, `OTEL_SDK_DISABLED`,`OTEL_SERVICE_NAME`,`OTEL_RESOURCE_ATTRIBUTES`,`OTEL_EXPORTER_OTLP_TIMEOUT`,`OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT`; Langfuse `LANGFUSE_PUBLIC_KEY`,`LANGFUSE_SECRET_KEY`,`LANGFUSE_URL`, and `LANGFUSE_INIT_PROJECT_*`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- Complete policy/settings precedence and process-only explicit plugin-root flag: unknown. `--no-profile` is extension isolation, not proof it suppresses all project hooks/skills.

## 4. Models and reasoning

- **[docs]** `--provider`, `--model`; `GOOSE_PROVIDER`, `GOOSE_MODEL`, `GOOSE_TEMPERATURE`, `GOOSE_MAX_TOKENS`, `GOOSE_PROVIDER__TYPE`,`GOOSE_PROVIDER__HOST`,`GOOSE_PROVIDER__API_KEY`; model context overrides `GOOSE_CONTEXT_LIMIT` (fallback 128000), `GOOSE_INPUT_LIMIT` (Ollama num_ctx). https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** `CLAUDE_THINKING_TYPE=adaptive|enabled|disabled`; adaptive default for 4.6+. `GEMINI3_THINKING_LEVEL=low|high`. Anthropic cache `GOOSE_CACHE_TTL=5m|1h` but headless/subagents/scheduled always 5m. Provider retry controls `BEDROCK_MAX_RETRIES`, `BEDROCK_INITIAL_RETRY_INTERVAL_MS`, `BEDROCK_BACKOFF_MULTIPLIER`, `BEDROCK_MAX_RETRY_INTERVAL_MS`; same suffixes for `DATABRICKS_`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** Feature-gated `goose local-models search|download|list|delete` supports local inference, not a universal remote model-list command. Unknown-model rejection timing unknown. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs

## 5. Conversations

- **[docs]** `--name/-n NAME` names a new session; `--session-id/--id ID` only valid with `--resume`; `--path` legacy extracts ID from filename, does not select transcript destination. Bare resume picks most recent user session. New-session chosen ID is rejected (`Cannot use --session-id without --resume`). `--no-session` skips persistence. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** `goose session --resume --fork` copies history; optional `--edit` edits conversation in `$VISUAL/$EDITOR/vi`; `--history` prints previous messages. `session list --format json --ascending --working_dir PATH --limit N`; `session remove [--session-id/--name/--regex]`; `session rename --session-id ID --new-name NAME`; export `--format markdown|json|yaml|html --output FILE`; import supports Goose JSON and Claude/Codex/Pi JSONL. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** Auto-compaction `GOOSE_AUTO_COMPACT_THRESHOLD` default 0.8 (0 disables), `GOOSE_AUTO_COMPACT_TOKEN_LIMIT` default 225000, earlier threshold wins, `GOOSE_TOOL_CALL_CUTOFF`. `GOOSE_DISABLE_SESSION_NAMING=true` avoids background naming model call. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- Exact current session DB filenames/locking and same-session writers: unknown; source explicitly calls a SessionManager rather than guaranteeing legacy JSONL is current storage.

## 6. Transcript and structured output

- **[docs]** `goose run --output-format json|stream-json` and `--stats`; `--quiet` emits only model response. Multi-format exports above; `session diagnostics --output PATH`. Exact event/result JSON schema and live transcript flush contract unknown in checked CLI source. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** `GOOSE_MAX_TOOL_RESPONSE_SIZE` spills oversized tool output to temporary file; OTel can include actual content only with explicit `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true` (default false). https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md

## 7. Hooks and events

- **[docs]** Plugins contain `hooks/hooks.json`: `{hooks:{Event:[{matcher:REGEX,hooks:[{type:'command',command:'${PLUGIN_ROOT}/...',timeout:30,on_failure:'allow'|'block'}]}]}}`. `sh -c`; plugin root also in environment. User `~/.agents/plugins/<name>`, project `.agents/plugins/<name>`, installed plugin directory. Bad matcher skips rule; bare `*` is invalid regex. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- **[docs]** Complete emitted inventory: `SessionStart`, `SessionEnd`, `Stop`, `UserPromptSubmit`, `PreToolUse`, `PreToolUseResult`, `PostToolUse`, `PostToolUseFailure`, `BeforeReadFile`, `AfterFileEdit`, `BeforeShellExecution`, `AfterShellExecution`. `SubagentStart`/`SubagentStop` are not emitted. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- **[docs]** Payload common `event`,`session_id`; optional event-specific `matcher_context`,`tool_name`,`tool_input`,`message`,`last_assistant_message`,`working_dir`,`tool_call_id`; PreToolUseResult adds `decision:allow|deny`,`policy_evaluated`,`blocked_by`,`reason`,`cause:policy_denial|hook_failure`. Matchers inspect tool name, prompt, file path or command depending event. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- **[docs]** Only PreToolUse and Stop can block: exit 2 + stderr or stdout `{decision:'block',reason}` (honored even nonzero exit). Stop block continues turn, capped by `GOOSE_STOP_HOOK_BLOCK_CAP` (default numeric cap unknown). Clean allow is exit 0 plus empty stdout or `{decision:'allow'}`; `{}` is **no decision**, potentially fail-closed. PreToolUse `on_failure:block` denies timeout/spawn/malformed output/no decision; default allows. Other events observe only, including BeforeReadFile/BeforeShellExecution and UserPromptSubmit. No documented input rewrite/permission grant bypass. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- **[docs]** PreToolUseResult is awaited despite observation-only; delivery not durable. Hook reasons not bounded/redacted; output cap and async option unknown. Disabled plugin names in user/project `.config/goose/settings.json` `disabledPlugins`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md

## 8. Plugins and extensions

- **[docs]** MCP extensions can add tools via process-scoped flags above; builtins include developer and others by name. Open Plugins packages with `plugin.json` and hooks discovered from user/project/installed directories. `goose plugin install URL [--auto-update]`, `plugin update NAME`; `skills list` is read-only inventory. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- General runtime JS plugin API for registering UI/providers/commands/agents: unknown; do not equate MCP extensions with in-process plugin callbacks.

## 9. Tools, permissions, and approvals

- **[docs]** `GOOSE_MODE=auto|approve|chat|smart_approve`, default auto. Developer tools schemas include `shell:{command,timeout_secs?}`, `write:{path,content}`, `edit:{path,before,after}`, `tree:{path,depth}`, `read_image:{source,crop?}`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md
- **[docs]** `--container ID` runs stdio/builtin extensions inside existing Docker container, requiring extensions and Goose for builtins inside. This is an explicit tool-execution boundary, not proof hooks or main process are containerized. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- External approvals/questions via ACP possible as protocol integration; exact implemented permission message schema and allow/deny policy files unknown in checked sources.

## 10. Subagents and delegation

- **[docs]** Delegate/sub-recipe support; `GOOSE_MAX_CONCURRENT_SUBAGENTS` default 5, positive only; `GOOSE_SUBAGENT_MAX_TURNS`, `GOOSE_SUBAGENT_PROVIDER`,`GOOSE_SUBAGENT_MODEL`; child recipe `settings.goose_provider/goose_model` overrides subagent env; explicit delegate max_turns/recipe wins over env. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** Native `review` discovers `.agents/checks/*.md` and `.agents/REVIEW.md`; default orchestrator launches up to four subprocess checks, `--no-orchestrate` restores model-driven delegate. Other flags `--model`,`--provider`,`--override-model`,`--turn-limit`,`--dry-run`,`--prompt`,`--instructions`,`--files`,`--check-filter`,`--check-scope`,`--checks-only`,`--summary-only`,`--severity`. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- Definitive disable-all-delegation switch unknown (setting concurrency 0 does not disable; invalid falls back).

## 11. MCP

- **[docs]** Process flags support stdio and streamable HTTP, configurable timeout and stdio env; `--with-builtin` for bundled servers, `goose mcp NAME` serves bundled MCP. Hidden `mcp-probe COMMAND --script JSON|-` starts MCP without LLM and supports ListTools/ListPrompts/ListResources/CallTool plus elicitation/oauth/protocol-version test configuration. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- **[docs]** OAuth callback random unless `GOOSE_OAUTH_CALLBACK_PORT` fixed; MCP callback `/oauth_callback`. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- Full resource/prompt/elicitation surfacing and server-push-to-agent semantics unknown; list capability alone does not prove unsolicited prompt injection.

## 12. Control from outside

- **[docs]** `goose acp` serves ACP over stdio, flags `--with-builtin`, `--enable-scheduler`. Feature-gated `goose serve` exposes ACP HTTP/WebSocket: `--host` (127.0.0.1), `--port` (3284), `--tls`,`--tls-cert-path`,`--tls-key-path`,`--platform cli|desktop`,`--with-builtin`,`--dangerously-unauthenticated`, repeat `--allowed-origin`,`--enable-scheduler`, optional `--roam`. Secret `GOOSE_SERVER__SECRET_KEY` required unless explicit bypass. TLS env `GOOSE_TLS`,`GOOSE_TLS_CERT_PATH`,`GOOSE_TLS_KEY_PATH`; startup prints `GOOSED_CERT_FINGERPRINT=...`. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- **[docs]** Feature-gated peer-to-peer `goose roam`; gateway status/start/stop/pair (e.g. Telegram), scheduling add/list/remove/sessions/run-now. Terminal integration `goose term init SHELL`, `term run PROMPT`, `term info` persists per-terminal context; `--dangerous-include-history` can expose shell-history secrets. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- Detailed ACP methods/capabilities, attaching ordinary CLI TUI to a remote server and interactive steering/interrupt guarantees: unknown.

## 13. Status from outside

- **[docs]** Hooks SessionStart/Stop/SessionEnd, PreToolUseResult and tool success/failure allow external event logging. `GOOSE_CLI_BELL` distinguishes neither finish nor permission by itself. ACP and OTel are richer integration routes; exact live-state enum unknown. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md

## 14. Usage, cost, and limits

- **[docs]** `run --stats`; `GOOSE_CLI_SHOW_COST=true`; `term info` compact token/model readout; OTel metrics/traces and Langfuse configuration. `--max-turns` and `--max-tool-repetitions` bound loops; context/output/token limits above. Provider quota API and exact stats JSON fields unknown. https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md

## 15. Authentication

- **[docs]** Provider-specific APIs/OAuth; system keyring default, `GOOSE_DISABLE_KEYRING` (presence, not boolean value) disables. File fallback `~/.config/goose/secrets.yaml` or Windows `%APPDATA%\Block\goose\config\secrets.yaml`. `goose info --verbose` shows config; `info --check` tests connection, so is not an offline/auth-only probe. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
- Uniform expired-auth check without model/provider call: unknown.

## 16. Running many instances

- **[docs]** `GOOSE_PATH_ROOT` isolates file roots, but keyring may remain shared unless disabled; unique server ports and OAuth callback ports required. Default OAuth callback random reduces collision. Extensions may spawn own processes. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md
- Session DB locking, same-project mutation safety, memory budget and shared account quotas unknown.

## 17. Anything else a program driving this harness should know

- **[docs]** Disable auto session naming when extra model calls matter; arbitrary nonzero hook failure is not a deny, and “Before” hook names do not imply control authority. Several server/review/scheduler features are compile-time gated, so local help is necessary before relying on main-source inventory. https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/environment-variables.md https://raw.githubusercontent.com/block/goose/main/documentation/docs/guides/context-engineering/hooks.md https://raw.githubusercontent.com/block/goose/main/crates/goose-cli/src/cli.rs
