# Gemini CLI

## 1. Overview

- **[docs]** Google Gemini CLI, Apache-2.0, Node/TypeScript, npm package `@google/gemini-cli`; install via npm or published native/package-manager mechanisms. Public source `main/package.json` checked at survey time reports `0.67.0-nightly.20261020.gb041de017` (source version, **not a locally verified installed version**). Sources: https://github.com/google-gemini/gemini-cli and https://github.com/google-gemini/gemini-cli/blob/main/package.json
- Local installed version: unknown; unlike the other four assigned harnesses, Gemini was not asserted installed and no Gemini executable was run. No installation/model call occurred.
- **[docs]** Official site currently warns Gemini CLI was replaced by Antigravity CLI on June18,2026 for unpaid-tier/Google One users, and isn't optimized for Gemini4 Argon. This is account/product eligibility information, not a claim CLI source vanished. Source: https://geminicli.com/docs/cli/headless/
- Stable/preview/nightly release channels exist; precise release guarantee/adoption numbers: unknown.

## 2. Interactive terminal interface

- **[docs]** Interactive terminal REPL; `--screen-reader` accessible mode, prompt-interactive `-i`; notification opt-in `general.enableNotifications:true` (default false) sends OSC9 and falls back BEL in unsupported terminals. Action-required and session-complete notifications distinct. Sources: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md and https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/notifications.md
- **[docs]** Settings include theme/layout/thinking/spinner/terminal title, experimental OSC52 copy/paste. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/settings.md
- Exact alternate-screen default, title escape format, OSC9;4/7501, mouse protocol and resize bugs for source version: unknown. No PTY capture performed.

## 3. Launch and configuration

- **[docs]** CLI/source flags: initial query, `-p/--prompt` forces headless, `-i/--prompt-interactive`, `-m/--model`, `-s/--sandbox`, `--skip-trust`, `--approval-mode default|auto_edit|yolo|plan`, `--policy`, `--admin-policy`, `--acp`, deprecated `--experimental-acp`, `--allowed-mcp-server-names`, deprecated `--allowed-tools`, `-e/--extensions`, `--list-extensions`, `--list-models`, `--resume`, `--session-file`, `--session-id`, `--list-sessions`, `--delete-session`, `--include-directories`, `--screen-reader`, `--output-format text|json|stream-json`, debug/help/version/worktree flags. Source: https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/config.ts
- **[docs]** Layer precedence low→high: builtins; system-defaults; user; project; system overrides; environment; CLI. File paths Linux `/etc/gemini-cli/{system-defaults,settings}.json`, macOS `/Library/Application Support/GeminiCli/{system-defaults,settings}.json`, Windows `C:\ProgramData\gemini-cli\...`; user `~/.gemini/settings.json`, project `.gemini/settings.json`. Env redirects `GEMINI_CLI_SYSTEM_DEFAULTS_PATH`, `GEMINI_CLI_SYSTEM_SETTINGS_PATH`. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md
- **[docs]** `GEMINI_CLI_HOME` is a **parent home directory**, CLI creates `.gemini` inside it, not path to .gemini itself. Isolates user config/storage. `GEMINI_CLI_TRUSTED_FOLDERS_PATH` isolates trust file; `GEMINI_CLI_TRUST_WORKSPACE=true` trusts for this run. `--skip-trust` sets equivalent env. Source: same config docs and config.ts.
- **[docs]** JSON/extension string substitutions `$VAR`, `${VAR}`, `${VAR:-DEFAULT}`; .env files discovered and extensions can load their own .env. Settings controlling .env loading (`security.folderTrust.enabled`, `advanced.ignoreLocalEnv`, `advanced.excludedEnvVars`, `tools.sandbox`) evaluate before dotenv and can only reference shell env. Source: configuration reference.
- **[docs]** Model/env auth/base URL/telemetry overrides detailed below; `GEMINI_SYSTEM_MD=true|1` uses project `.gemini/system.md`, path string selects replacement, false|0/unset uses builtin. `GEMINI_WRITE_SYSTEM_MD` **writes a file** and is not read-only inspection. Source: same reference.
- No generic inline JSON settings env override found; custom user home/system paths, explicit policy files and CLI options are established per-process routes. Separate extension directories without installation require isolated home or configured discovery; a direct per-process arbitrary extension-path flag was not established.

## 4. Models and reasoning

- **[docs]** `--model` accepts model alias/full name; `GEMINI_MODEL` overrides hardcoded default; `--list-models` exists in current source despite cheatsheet omission. Historical cheatsheet maps auto/pro to 2.5-pro/3-pro-preview depending preview flags, flash/flash-lite to 2.5 families; do **not** hardcode as current catalog. Sources: config.ts and https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md
- **[docs]** `modelConfigs.overrides[].match` scopes model configuration; `modelConfig.generateContentConfig` carries temperature/thinking-related provider parameters. ACP unstable_setSessionModel changes current model. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/core/subagents.md and https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/acp/acpRpcDispatcher.ts
- **[docs]** Custom `GOOGLE_GEMINI_BASE_URL` (API-key auth) / `GOOGLE_VERTEX_BASE_URL` (Vertex), HTTPS except loopback; `GOOGLE_GENAI_API_VERSION` chooses API version. Source: configuration reference.
- Unknown model behavior, complete reasoning effort enum/current account catalog: unknown; no model requests made.

## 5. Conversations

- **[docs]** Source supports `--session-id` chosen new ID (described as UUID but validator accepts any nonempty `[a-zA-Z0-9-_]+`), `--session-file` JSON load, `--resume [latest|index|full ID]`; the three are mutually exclusive. Bare --resume means latest. Source: https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/config.ts
- **[docs]** Automatic JSON transcripts in `~/.gemini/tmp/<project_hash>/chats/`, project-root hash; store prompts/responses/tools inputs+outputs/token usage/thought summaries. `--list-sessions`, `--delete-session index|id`, interactive browser `/resume`/`/chat`; named checkpoints `/resume save TAG`, `/resume resume TAG`, `/resume list`, compatibility `/chat` and `/resume checkpoints`. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/session-management.md
- **[docs]** Retention default enabled/maxAge30d/minRetention1d/maxCount undefined; deletion removes plans/task trackers/tool output/activity logs too. `model.maxSessionTurns` default -1 unlimited; hit limit stops interactive requests and headless errors. Source: same session reference.
- **[docs]** Plan artifacts default `~/.gemini/tmp/<project>/<session-id>/plans/`; extension plan.directory fallback, overridden by user setting. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/extensions/reference.md
- Fork/export explicit stable CLI flags and same-session multi-writer locks: unknown. Checkpoint/rewind and worktree creation aren't equivalent to forking a chosen session identity.

## 6. Transcript and structured output

- **[docs]** Headless when -p or non-TTY; `--output-format json` returns `{response:string,stats:object,error?:object}`. stream-json is JSONL events `init` (session/model), `message`, `tool_use`, `tool_result`, `error`, `result` (aggregated/per-model stats). Source: https://geminicli.com/docs/cli/headless/
- **[docs]** Exit codes headless: 0 success,1 general/API failure,42 input error,53 turn-limit exceeded. Source: same headless reference.
- **[docs]** Full JSON session file and transcript_path in hooks can be read externally during operation; rewriting a whole JSON file is not an append-only tail protocol. Todos/plans and tool call updates provide richer task state than assistant prose. Sources: session and hook references.
- Exact stream event field schema beyond documented summaries, atomic transcript write guarantee, all plan/todo on-disk schemas: unknown.

## 7. Hooks and events

- **[docs]** Complete events in reviewed reference: `BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `BeforeModel`, `BeforeToolSelection`, `AfterModel`, `SessionStart`, `SessionEnd`, `Notification`, `PreCompress`. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/reference.md
- **[docs]** Settings `hooks.EVENT:[{matcher?,sequential?:boolean,hooks:[{type:"command",command,name?,timeout?,description?}]}]`; only command type supported; default timeout60000ms; groups parallel unless sequential true. stdin JSON, stdout **only final JSON**, stderr logs. Exit0 parse output,2 block with stderr reason,other warning continues. Source: same hooks reference.
- **[docs]** Common input `{session_id,transcript_path,cwd,hook_event_name,timestamp}`. Common output `{systemMessage?,suppressOutput?,continue?,stopReason?,decision?:allow|deny|block,reason?}`; support varies event. Source: same reference.
- **[docs]** BeforeTool input `tool_name,tool_input,mcp_context?,original_request_name?`; output deny blocks call but turn continues; hookSpecificOutput.tool_input merges overriding model args. AfterTool input adds `tool_response:{llmContent,returnDisplay,error?}`; deny hides/replaces output with reason, additionalContext appends, `tailToolCallRequest:{name,args}` immediately routes another call whose result replaces original. Source: same reference.
- **[docs]** BeforeAgent `{prompt}` can add additionalContext; deny discards user prompt from history, continue:false preserves prompt but stops turn. AfterAgent `{prompt,prompt_response,stop_hook_active}` deny requests another corrective turn with reason, continue:false stops; clearContext clears LLM history not UI. Avoid retry loops using stop_hook_active; global cap unknown. Source: same reference.
- **[docs]** BeforeModel `{llm_request:{model,messages,config}}` may override request or provide synthetic llm_response to skip model call. BeforeToolSelection controls `toolConfig.mode:AUTO|ANY|NONE` and allowedFunctionNames union; NONE wins; ignores decision/continue/systemMessage. AfterModel gets request+response and fires **each streaming chunk**, replacement affects current chunk only; deny/continue false abort. Source: same reference.
- **[docs]** Stable LLMRequest messages `{role:user|model|system,content:string}` filter nontext; config generation settings; toolConfig modes/names. LLMResponse candidates content `{role:model,parts:string[]}`, finishReason, usageMetadata.totalTokenCount. Source: same reference.
- **[docs]** SessionStart.source startup|resume|clear; context prepended to headless prompt / first interactive turn; decisions ignored (cannot block startup). SessionEnd.reason exit|clear|logout|prompt_input_exit|other, best effort not awaited/decisions ignored. Notification.type ToolPermission with message/details, observation only cannot grant approval. PreCompress.trigger auto|manual async advisory cannot stop compression. Source: same reference.
- Per-process hook injection without file: isolated GEMINI_CLI_HOME settings/system override file or extension hooks; generic command-line inline hook JSON unknown. Output-size/tail-call recursion hard caps: unknown.

## 8. Plugins and extensions

- **[docs]** Extensions under `<home>/.gemini/extensions`, manifest gemini-extension.json; select process `--extensions` list (default all); manage install/uninstall/list/update/enable/disable/link/new/validate/config. Installs copy source (not live link), explicit link for development. Manifest name/version/description, mcpServers, contextFileName, excludeTools, migratedTo, plan.directory and settings. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/extensions/reference.md
- **[docs]** MCP settings entry wins same-name extension server; extension trust field unsupported. `${extensionPath}` anchors bundled executable/files; command and args separate. excludeTools permits command-scoped patterns. settings entries `{name,description,envVar,sensitive}`; sensitive in keychain, nonsecret config in extension .env. Environments sanitized: safe HOME/PATH/TMPDIR plus explicitly declared envVar, not full inherited shell env. Source: same extension reference.
- **[docs]** Bundle TOML commands under commands/ (`gcs/sync.toml`→`/gcs:sync`), hooks/hooks.json (not manifest hooks), skills/*/SKILL.md, agents/*.md. No arbitrary in-process TUI renderer/provider registration API established; use MCP/hooks/ACP. Source: same reference.
- **[docs]** Documentation conflict: extension reference says changes require restart, CLI cheatsheet lists `/extensions reload`, `/commands reload`, `/agents reload`, `/skills reload`, `/memory reload`, `/mcp reload`. Treat actual release/source behavior as authority rather than assuming universal live reload. Sources: extension reference and https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md

## 9. Tools, permissions, and approvals

- **[docs]** Approval modes default prompts, auto_edit automatically edit, yolo all, plan read-only; policy engine TOML allow/deny/ask governs exact tools/commands/subagents. `--allowed-tools` deprecated; `--policy` and `--admin-policy` load per-process files/dirs. Untrusted folder forces default; security.disableYoloMode/admin.secureModeEnabled can refuse yolo. Source: config.ts.
- **[docs]** Builtins include file read/list/search/write/replace, run_shell_command, web_search/web_fetch, memory, todos/planning and delegate tools; default exact inventory is version/feature dependent. Tools core/allowed/exclude settings and extension excludeTools narrow exposure. Source: configuration reference.
- **[docs]** `--sandbox` / `GEMINI_SANDBOX=true|false|docker|podman|custom command`; macOS Seatbelt profiles `permissive-open` default write confinement/broad reads+network, restrictive-open, strict-open, strict-proxied; custom profile named sandbox-macos-NAME.sb in project .gemini. Source: configuration reference.
- **[docs]** ACP `session/request_permission` has structured choices and toolCall metadata. MCP approval toolCall._meta['gemini-cli'].mcp `{server_name,tool_name}` identifies origin, not a trust grant. Client can answer approvals directly. Notification hook cannot answer approvals; must policy/ACP/UI. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md
- Agent questions can be handled through interactive/ACP updates and permission/form mechanisms; full standalone user-question response schema: unknown.

## 10. Subagents and delegation

- **[docs]** Native local/remote subagents, automatic or @agent forced; builtins codebase investigator, CLI help, generalist, optional browser agent. Definitions `.md` under project/user agents and extension agents; frontmatter name/description/kind/local|remote/tools/model/temperature/max_turns/timeout_mins. Defaults max_turns30/time10min; omitted tools inherit parent, wildcards `*`, `mcp_*`, `mcp_server_*`. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/core/subagents.md
- **[docs]** `agents.overrides.NAME.enabled:false` disables; runConfig maxTurns/maxTimeMinutes limits; policy virtual tool name deny also restricts. Model overrideScope targets agent. Remote agents use separate server environments; exact A2A wire contract not read here: unknown. Source: same subagent reference.
- **[docs]** Browser agent optional Chrome DevTools MCP, isolated/existing session modes; maxActionsPerTask default100; macOS sandbox forces isolated/headless; container sandbox disables unless existing browser configured. Source: same reference.
- Exact background task/team shared message API and universal delegation-off flag: unknown; deny each exposed agent via policy/overrides.

## 11. MCP

- **[docs]** Client stdio/Streamable HTTP/SSE; per-server config command/args/env/cwd or httpUrl/url, timeout/trust/includeTools/excludeTools, OAuth; server allowlist flag selects process servers. Resources/prompts/tools supported and prompts surface as commands. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md
- **[docs]** command=stdio, url=legacy SSE, httpUrl=Streamable HTTP; timeout default600000ms; trust defaultfalse (true bypasses server confirmations); excludeTools wins includeTools. env substitutes $VAR/${VAR} on all OS and %VAR% Windows; absent→empty. authProviderType service_account_impersonation adds targetAudience OAuth client ID and targetServiceAccount email. Source: same MCP reference.
- **[docs]** Tool hook names `mcp_<server>_<tool>`; settings mcpServers config takes precedence over extension same name; extension may not set trust. ACP initialize advertises mcpCapabilities http:true,sse:true and prompt image/audio/embeddedContext true; ACP session creation can receive IDE-provided servers. Sources: hooks/extension references and acpRpcDispatcher.ts.
- MCP server unsolicited push beginning a new turn, complete elicitation support/current protocol version: unknown; catalog notification ≠ user prompt injection.

## 12. Control from outside

- **[docs]** **Current source prefers `gemini --acp`**; --experimental-acp deprecated, not only available flag. Persistent stdio JSON-RPC2.0 ndjson; agent-side SDK connection; supports multiple isolated sessions via session manager. Source: https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/config.ts and https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/acp/README.md
- **[docs]** Implementation methods `initialize`, `authenticate`, `newSession`, `loadSession`, `prompt`, `cancel`, `setSessionMode`, `unstable_setSessionModel`; wire ACP methods session/new, session/load, session/prompt, session/cancel, session/set_mode, session/set_model via SDK mapping. Session prompt/update/tool/permission channels are structured; cancel calls cancelPendingPrompt. Sources: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md and https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/acp/acpRpcDispatcher.ts
- **[docs]** initialize returns protocolVersion, authMethods, agentInfo{name:'gemini-cli',title:'Gemini CLI',version}, loadSession:true and prompt/MCP capabilities. Bad session ID returns -32602 `Session not found: ...`. authenticate methodId parsed as AuthType, optional `_meta['api-key']`, `_meta.gateway {baseUrl?,headers?}`; malformed gateway -32602, auth errors -32000; **persists security.auth.selectedType in user settings** and may clear cached credentials on method change. Not a read-only probe. Source: acpRpcDispatcher.ts.
- **[docs]** ACP filesystem proxy/client permission mechanisms integrate editor access; do not infer all external commands are sandboxed just because file reads proxy. Source: ACP guide and implementation README.
- No separately documented HTTP server/TUI attach-to-same-running-ACP session/queue CLI found. Running interactive PTY can be controlled through terminal input, but robust external steering should use ACP rather than scraping. Concurrent in-flight prompt steering semantics: unknown.

## 13. Status from outside

- **[docs]** Headless init/message/tool_use/tool_result/result/error exposes lifecycle; ACP session updates and pending permission requests identify work and waiting; cancel response and prompt stop reason indicate completion. Hook BeforeAgent/AfterAgent and Notification ToolPermission are additional signals; notification is advisory. Sources: headless/hooks/ACP references.
- **[docs]** Successful structured output can coexist with warnings/error events; inspect final result and process exit code, not text alone. Exact busy/idle query endpoint and durable reconnect cursor: unknown.

## 14. Usage, cost, and limits

- **[docs]** Headless JSON stats includes tokens/API latency, stream result aggregates per-model usage; transcript retains input/output/cache counts. model.maxSessionTurns default-1, exit53 if exceeded headless. Sources: headless/session references.
- **[docs]** Telemetry env: GEMINI_TELEMETRY_ENABLED, TRACES_ENABLED, TARGET local|gcp, OTLP_ENDPOINT, OTLP_PROTOCOL grpc|http, OTLP_HEADERS (JSON or comma k=v, merges and overrides settings, fallback OTEL_EXPORTER_OTLP_HEADERS), LOG_PROMPTS, OUTFILE, USE_COLLECTOR; enabled flags true|1 versus others false. Local telemetry captures ACP events; may contain sensitive prompts if logging enabled. Source: configuration/ACP references.
- Accurate dollar cost API, per-process spend cap, context occupancy and shared rate-limit query without model call: unknown. Quotas depend account/auth/product eligibility.

## 15. Authentication

- **[docs]** Google OAuth sign-in caches locally; Gemini API key GEMINI_API_KEY; Vertex ADC/service account GOOGLE_APPLICATION_CREDENTIALS or express GOOGLE_API_KEY, normal Vertex GOOGLE_CLOUD_PROJECT and GOOGLE_CLOUD_LOCATION. Headless prefers env auth; existing cached OAuth may be reused. Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/get-started/authentication.mdx
- **[docs]** GEMINI_CLI_HOME redirects credential/config parent home; system environment/API key vars should be unset if choosing ADC instead. ACP authenticate can initiate/refetch credentials and writes selectedType; do not call to merely ask status. Sources: auth guide/configuration/acpRpcDispatcher.ts.
- Exact credential filenames/keychain fallback and stable `auth status` no-model/no-refresh command: unknown; no such CLI subcommand established in reviewed source. Local file existence alone not proof of unexpired/revocation-free login.

## 16. Running many instances

- **[docs]** Distinct GEMINI_CLI_HOME roots, session IDs, project worktrees and explicit policy/extension scope isolate processes. Default same-home settings/auth/extensions/project hash storage are shared; `--worktree` requires experimental.worktrees:true. Source: configuration and config.ts.
- **[docs]** ACP session manager supports multiple sessions in one process; remote subagent servers and Chrome browser modes introduce additional ports/processes. Browser existing mode requires intentional remote debugging endpoint. Source: ACP README/subagent guide.
- Per-instance memory, file locking, safe concurrent same-session writes, shared token refresh contention and client concurrency limits: unknown; not benchmarked.

## 17. Anything else a program driving this harness should know

- **[docs]** Cheatsheet is behind implementation: lists experimental-acp but source prefers acp; omits session-id/session-file/list-models/policy/admin-policy. Session-id described UUID but validator broader. Use version-matched source/--help, not docs absence, for supported flags. Source: config.ts and CLI cheatsheet.
- **[docs]** Surface tagging `GEMINI_CLI_SURFACE` changes API User-Agent reporting; GEMINI_CLI_IDE_PID selects explicit IDE process; GOOGLE_GEMINI_BASE_URL/GOOGLE_VERTEX_BASE_URL require HTTPS except loopback; .env and extension environment sanitation can make child env differ from launcher. Source: configuration reference.
- **[verified]** Public source snapshots retained in sources/gemini-*.txt with sources/urls.json; no installation or local Gemini/global config/credentials inspection.
