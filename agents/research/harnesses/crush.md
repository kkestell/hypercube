# Crush

## 1. Overview

- **[verified]** Research 2026-10-30; `crush` not on PATH, no execution/install/model calls. Runtime version unknown.
- **[docs]** Charm's Go terminal coding agent, **FSL-1.1-MIT**, not simply MIT. Latest release API `v0.99.0` (2026-10-29); current main source/docs checked, may be ahead of release. Installs Homebrew `charmbracelet/tap/crush`, npm `@charmland/crush`, `go install github.com/charmbracelet/crush@latest`, release binaries/packages, Winget/Scoop/Nix/pkg. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://api.github.com/repos/charmbracelet/crush/releases/latest
- **[docs]** Extra selection by repo-star proxy: Cline71,082, Continue36,379, Crush28,890 selected; Qwen28,840, Kilo27,807, archived Python Kimi11,422 rejected. 50-star lead over Qwen is narrow/time-dependent; Cline/Continue totals include IDE audiences. No Factory Droid comparable public metric established; terminal-only usage and absolute global top three unknown. https://api.github.com/repos/cline/cline https://api.github.com/repos/continuedev/continue https://api.github.com/repos/charmbracelet/crush https://api.github.com/repos/QwenLM/qwen-code https://api.github.com/repos/Kilo-Org/kilocode https://api.github.com/repos/MoonshotAI/kimi-cli
- Release cadence/active users unknown. Kimi archived Python repo points to replacement `MoonshotAI/kimi-code`; not treated as current CLI.

## 2. Interactive terminal interface

- **[docs]** Bubble Tea v2 TUI, Ctrl+L model picker, Ctrl+P command palette; themes preview live, Ctrl+E edit selected theme, Enter/Ctrl+S save, Escape cancel. Builtin themes `charmtone-panther`, `gruvbox-dark`, `keanu`; global themes JSON. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go
- **[docs]** Notifications on permission/completion only when terminal unfocused and supports focus reporting. `option notifications auto|native|osc|bell|disabled`; auto native locally/OSC over SSH. Exact OSC9/7501/title/mouse/alternate-screen sequences unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** Startup writes `ansi.SetIndeterminateProgressBar` then `ansi.ResetProgressBar` on stderr TTY recognized via WT_SESSION or TERM_PROGRAM containing ghostty/iterm2/rio; noninteractive server run also progress on stderr TTY. Source uses named ANSI constants (exact bytes not independently inspected; do not claim captured OSC9;4). https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/run.go
- **[docs]** Clipboard Linux/BSD needs wl-copy/wl-paste on Wayland or xclip/xsel X11; macOS/Windows native. Native OS notifications unavailable illumos but OSC/bell work; Oracle Solaris SQLite needs build tag `sqlite3_dotlk`. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md

## 3. Launch and configuration

- **[docs]** Global `--cwd/-c`, `--data-dir/-D`, `--debug/-d`, `--host/-H`, counted `--yolo/-y` (one vs two materially differs), hidden repeatable/string-slice `--channels server:webhook`; interactive `--session/-s ID`, `--continue/-C` mutually exclusive. `crush run/r PROMPT` reads piped stdin + blank line + argument; run flags `--quiet/-q`,`--verbose/-v`,`--model/-m`,`--small-model`,`--reasoning-effort`,`--session/-s`,`--continue/-C`. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/run.go
- **[docs]** Current configuration is executable Bash `crushrc`: project `.crushrc` priority1, `crushrc` priority2, global `~/.config/crush/crushrc` priority3 (XDG honored). Builtin embedded shell identical across platforms; old JSON deprecated but supported. `CRUSH_GLOBAL_CONFIG`,`CRUSH_GLOBAL_DATA` override global locations. JSON state `~/.local/share/crush/crush.json`, Windows `%LOCALAPPDATA%\crush\crush.json`; no crushrc execution from data dirs. **Both crushrc and JSON `$(...)` are trusted executable code.** https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** `CRUSH_CLIENT_SERVER=true` enables client-server architecture; otherwise local embedded backend. `CRUSH_SERVER_READY_TIMEOUT` Go duration default10s; `CRUSH_DISABLE_PROVIDER_AUTO_UPDATE=1`, `CATWALK_URL`, `CRUSH_DISABLE_METRICS=1`, `DO_NOT_TRACK=1`, `CRUSH_SKILLS_DIR`, XDG paths. Top-level JSON `env` applies before provider setup with `$VAR`/`$(command)` expansion. Provider auth env inventoried section15. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** `option context/global-context/skill` examples: `option global-context-path PATH` repeated files or recursive Markdown directories, `option skill-path PATH...`, `option disable-skill NAME`, `option initialize-as AGENTS.md`, `option debug true`, `option debug-lsp true`, `option provider-auto-update false`, attribution options. Global context defaults `~/.config/crush/CRUSH.md`, `~/.config/AGENTS.md`; repo `.gitignore` and `.crushignore` respected. `crush schema` emits config schema (source command registered). Full embedded Bash builtin command schema and per-process inline config flag unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go

## 4. Models and reasoning

- **[docs]** run `--model MODEL|PROVIDER/MODEL`, `--small-model`, `--reasoning-effort`; unknown model returns `large|small model "..." not found`; ambiguous names demand provider prefix; case-insensitive model matching, provider exact. Unsupported effort rejected listing accepted values. Continue restores provider/model of last non-summary assistant message if still available, unless explicit override. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/run.go
- **[docs]** Catwalk builtin/provider auto-update and local model autodiscovery. Config `provider add NAME --type responses|completions|messages --base-url URL --api-key KEY --extra-header NAME VALUE`; legacy openai/openai-compat/anthropic auto-migrate. Local provider types `llamacpp`,`omlx`,`lmstudio`,`litellm`,`ollama`; `--discover-models true` merges, explicit fields win. `model add provider/model --name --context-window --default-max-tokens --thinking toggleable --supports-images --price-input --price-output --price-cache-create --price-cache-hit`. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- Standalone `models` command registration in current root not established; Ctrl+L and provider API inventory supported, `show model list` exact CLI unknown.

## 5. Conversations

- **[docs]** Fresh session each invocation by default; `--session ID`/`--continue` both TUI/run; full UUID or session hash prefix supported, ambiguous prefix error. run rejects continuation of child sessions. `crush session list|show ID|last|delete ID|rename ID TITLE`, each `--json`; aliases `sessions`, list aliases as source-defined (not exhaustively checked). https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/run.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/session.go
- **[docs]** SQLite in configured data directory, root opens DB through db.Connect and logs `<data-dir>/logs/crush.log`; default project `.crush/`. REST create/update/delete/history/messages; caller-chosen ID and fork CLI unknown. Session fields section14. `/compact` manual compaction hook trigger; automatic compaction supported, exact threshold unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md

## 6. Transcript and structured output

- **[docs]** `crush run` emits text; no JSON flag in checked run definition. `session show --json` plus REST history/messages are structured reads. SSE event endpoint; source `RunComplete` authoritative end marker fields `session_id,run_id?,message_id,text?,error?,cancelled?`. It occurs exactly once per top-level turn after message updates; compare caller-generated unique run_id, **not just session_id** when queueing behind busy work. Message-finish/tool_use does not mean whole run complete. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/run.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go
- **[docs]** Agent events `type:error|response|summarize`, `message`, optional error serialized string, run_id/session_id/session_title/progress/done/aws_sso_command/aws_sso_url. Session has todos structured `{content,status,active_form}`. Logs `crush logs [--tail N] [--follow]`, default1000 lines, debug expands logging. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/agent.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/session.go https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- Standalone append-only transcript format/file flush and full SSE event-name map unknown in inspected extracts.

## 7. Hooks and events

- **[docs]** Complete13 events: `PreToolUse`,`PostToolUse`,`PostToolUseFailure`,`PermissionRequest`,`PermissionResult`,`SessionStart`,`SessionEnd`,`TurnStart`,`TurnEnd`,`StopFailure`,`Interrupt`,`PreCompact`,`PostCompact`. Names case-insensitive/snake-case accepted, unknown config error. **Only top-level** tool/turn events; subagents don't fire them, outer delegate call does. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** `hook add EVENT --matcher REGEX --command STRING --name NAME --timeout SECONDS`; JSON flat `{hooks:{EVENT:[{matcher?,command,name?,timeout?}]}}`, unlike Claude nested hook groups. Project precedence, relative command resolved cwd not config directory. Embedded POSIX shell; shebang dispatch via os/exec with basename PATH fallback; .ps1 not autodispatched, use powershell/pwsh -File. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** All matching commands dedup by command, execute parallel, aggregate in config order. Default30sec, timeout fail-open, ~1sec interpreter grace then abandon; subprocess killed context. SessionEnd shared2sec after turn cancel. Only command hooks, no http/prompt/agent or async flag. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** Payload common `event,hook_event_name,session_id,cwd`; tool events `tool_name,tool_input,tool_use_id`; post `tool_response:string`, failure `error:string`, permission-result `decision:granted|denied|cancelled`; TurnStart/Interrupt common-only; TurnEnd `last_assistant_message`; StopFailure `error`; SessionStart `source:startup|resume`, SessionEnd `reason:'exit'`, compaction `trigger:auto|manual`. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** Env `CRUSH=1`,`AGENT=crush`,`AI_AGENT=crush`,`CRUSH_EVENT`,`CRUSH_TOOL_NAME`,`CRUSH_SESSION_ID`,`CRUSH_CWD`,`CRUSH_PROJECT_DIR`,`CLAUDE_PROJECT_DIR`,`CRUSH_TOOL_INPUT_COMMAND`,`CRUSH_TOOL_INPUT_FILE_PATH`; tool-specific only appropriate events. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** Output envelope `{version?:1,decision?:allow|deny|null,halt?,reason?,context?:string|string[],updated_input?,updated_output?}`. Pre allows bypass permission dialog; deny current call retry allowed; halt entire turn; input shallow merge config-order last-wins, ignored if denied/halted. Post updated_output replaces text last-wins; deny reason becomes feedback (tool already ran); failure accepts halt/context. PreCompact halt/deny/exit2 prevents compaction, automatic blocked ends turn. All other events observe-only, **PermissionRequest cannot approve**, TurnEnd cannot re-enter loop. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- **[docs]** Exit0 JSON; exit2 blocks with stderr reason, exit49 halts whole turn; other errors nonblocking but valid JSON still applied. Any deny>allow>null, halt sticky, reasons/context concatenate config-order. Claude forms `hookSpecificOutput.permissionDecision/permissionDecisionReason/updatedInput/updatedToolOutput/additionalContext`, `decision:block`, `continue:false/stopReason` accepted. Output size cap unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md

## 8. Plugins and extensions

- **[docs]** MCP/LSP and Agent Skills rather than established general in-process plugin API. Global skills `CRUSH_SKILLS_DIR`, XDG agents/skills and crush/skills, `~/.agents/skills`, `~/.claude/skills`, Windows localappdata equivalents, options.skills_paths; project `.agents/skills`,`.crush/skills`,`.claude/skills`,`.cursor/skills`. SKILL.md `user-invocable:true` command palette, `disable-model-invocation:true` hides automatic selection. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** LSP `lsp add NAME --command --args ... --env NAME VALUE`; providers/models register via crushrc builtins. Runtime custom UI/tool/plugin registration, dynamic unload and process-only plugin dirs unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md

## 9. Tools, permissions, and approvals

- **[docs]** `permissions allow view ls grep edit mcp_NAME_TOOL`; `permissions deny bash sourcegraph` hides builtin tools. Bash provably inert commands may run unprompted, but redirection/assignment/substitution/pipeline/mutating args trigger normal permission. `-y` autoapprove while exec-time dangerous-command blocklist remains; `-yy` sysadmin also drops blocklist. Modes normal|yolo|sysadmin; no OS sandbox proven by these modes. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go
- **[docs]** POST permissions/grant `{permission:PermissionRequest,action:allow|allow_session|deny}` returns `{resolved:boolean}`; false means already resolved by another client, not error. Full PermissionRequest nested shape not inspected. Questions request `{id,session_id,tool_call_id,questions:[{id,type,label?,question,description?,choices:[{id,label,description?}]}],confirm_title?,confirm_description?}`. Answer `{batch_request_id,responses:[{request_id,selected_ids?,fill_in_text?,yes?,notes?}]}` returns resolved; question-resolved notification `{batch_id,session_id}`. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go
- **[docs]** Integrated SSH/GPG pinentry answer/cancel APIs exist; pinentry request `{id,prompt,key:{card_number?,key_id?,fingerprint?,holder?},kind,retry_count?,error?}` never carries resolved secret. Credential answers must not be logged as transcript. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/server/endpoints.go

## 10. Subagents and delegation

- **[docs]** `task`, `agentic_fetch` and related subagents exist; hooks do not follow them internally. Tool deny can hide task but exhaustive delegation-tool restriction names/count/budget config unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md
- No verified native persistent agent teams/background task CLI.

## 11. MCP

- **[docs]** `mcp add NAME --type stdio|http|sse --command CMD --args ... --url URL --timeout SECONDS --disabled-tools NAME --env KEY VALUE --header NAME VALUE`; OAuth JSON `oauth:true`, `oauth_client_id`,`oauth_client_secret`,`oauth_callback_port`; client ID bypasses dynamic registration, absent attempts RFC7591. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** Sessionless known GitHub endpoints autodetected; `sessionless:true`/`--sessionless true` avoids subscriptions/listen; tradeoff no live tool/prompt/resource-list-changed push. `false` forces default subscription. Resources/prompts native APIs: read `{name,uri}`; get prompt `{client_id,prompt_id,args:map<string,string>}` returns `{prompt}`. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/requests.go
- **[docs]** Hidden `--channels server:webhook`, workspace channels config indicates MCP channel opt-in for push; exact channel event-to-agent schema/security and elicitation support unknown in checked code. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go

## 12. Control from outside

- **[docs]** Main uses `crush server --host/-H` (not README's illustrative `serve` spelling), host supports HTTP/TCP/Unix socket, platform named-pipe code also present; `CRUSH_CLIENT_SERVER=true` then ordinary TUI/run connects using `--host`. Auto detached server startup, health GET `/v1/health`, version/build ID comparison; stale server asks shutdown_if_idle, busy server reused. `GET /v1/docs/` HTML and OpenAPI3.1 JSON docs route implementation exists; exact spec URL path not inferred. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/server.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/server/docs.go
- **[docs]** Request message `{hidden_user_message?,session_id,run_id?,channel?,prompt,attachments?}`; host context `{open_files?,active_file?,line?}` (1-based) accompanies prompts. Session note `{text}` adds context for next prompt **without starting run**. Shell direct `{session_id,command,term_width?}` response `{output,exit_code}`. APIs can queue/list/clear prompts, cancel specific session, summarize, switch active agent `{agent_id}`, initialize `{interactive}`. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/requests.go
- **[docs]** Endpoint inventory below: prefix `W=/v1/workspaces/{id}`, `S=W/sessions/{sid}`, `A=W/agent/sessions/{sid}`. Every path/method is from source, not guessed. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/server/endpoints.go

```text
GET /v1/health; GET /v1/version; POST /v1/control; GET /v1/config
DELETE /v1/clients/{client_id}
GET,POST /v1/workspaces; GET,DELETE W
POST W/current-session; GET,POST W/theme; GET,POST W/host-context
GET W/config; GET W/providers; GET W/events
GET,POST W/sessions; GET,PUT,DELETE S
GET S/history; GET S/messages; GET S/messages/user; GET W/messages/user
GET S/filetracker/files; POST W/filetracker/read; GET W/filetracker/lastread
GET W/lsps; GET W/lsps/{lsp}/diagnostics; POST W/lsps/start; POST W/lsps/stop
POST W/permissions/mode; POST W/permissions/grant
POST W/questions/answer; POST W/questions/cancel
POST W/pinentry/answer; POST W/pinentry/cancel; POST W/ssh/answer; POST W/ssh/cancel
GET,POST W/agent; POST W/agent/init; POST W/agent/update; POST W/agent/main
GET A; POST A/cancel; GET A/prompts/queued; GET A/prompts/list
POST A/prompts/clear; POST A/notes; POST A/summarize; POST A/shell
GET W/agent/default-small-model
POST W/config/set; POST W/config/remove; POST W/config/model
POST W/config/compact; POST W/config/provider-key
POST W/config/import-copilot; POST W/config/refresh-oauth
GET W/project/needs-init; POST W/project/init; GET W/project/init-prompt
GET W/git/branch; GET W/skills; POST W/skills/read
POST W/mcp/refresh-tools; POST W/mcp/read-resource
GET W/mcp/prompts; POST W/mcp/get-prompt; GET W/mcp/states
GET W/mcp/pending-auth; GET W/mcp/auth-url; POST W/mcp/auth
POST W/mcp/refresh-prompts; POST W/mcp/refresh-resources
POST W/mcp/docker/enable; POST W/mcp/docker/disable
GET,POST W/mcp/disabled; GET W/mcp/enabled
POST W/mcp/config-disabled; POST W/mcp/start
```

- **[docs]** Config requests `{scope,key,value}` set, `{scope,key}` remove, model `{scope,model_type,model}`, compact `{scope,enabled}`, provider key `{scope,provider_id,kind:string|oauth,api_key}`; OAuth refresh `{scope,provider_id}`. Server auth boundary and default socket address unknown; do not expose TCP casually. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/requests.go

## 13. Status from outside

- **[docs]** AgentInfo `{is_busy,is_ready,model,model_cfg}`; REST session `is_busy` computed from coordinator, `attached_clients` counts clients with live SSE stream viewing session (not hold-only). Session SSE path does **not** populate is_busy; compute activity from other events or REST. Permission/question events distinct from work; RunComplete errors/cancelled distinct. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/proto.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/session.go
- **[docs]** Hooks TurnStart/TurnEnd/Interrupt/StopFailure and permission events facilitate external observability but miss subagent internals. https://raw.githubusercontent.com/charmbracelet/crush/main/docs/hooks/README.md

## 14. Usage, cost, and limits

- **[docs]** Session JSON `{id,parent_session_id,title,message_count,prompt_tokens,completion_tokens,summary_message_id,cost,todos?,channel?,created_at,updated_at,is_busy,attached_clients}`. Cost is float, token counts integer, timestamps int64. `crush stats` registered; exact flags unknown. Custom model price inputs/cache/context fields above. Rate-limit protocol/hard spend cap unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/proto/session.go https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go

## 15. Authentication

- **[docs]** Provider env supported: `HYPER_API_KEY`,`ANTHROPIC_API_KEY`,`OPENAI_API_KEY`,`VERCEL_API_KEY`,`GEMINI_API_KEY`,`ZAI_API_KEY`,`MINIMAX_API_KEY`,`SYNTHETIC_API_KEY`,`HF_TOKEN`,`CEREBRAS_API_KEY`,`OPENROUTER_API_KEY`,`IONET_API_KEY`,`ALIBABA_SINGAPORE_API_KEY`,`ALIBABA_US_API_KEY`,`GROQ_API_KEY`,`AVIAN_API_KEY`,`OPENCODE_API_KEY`,`VERTEXAI_PROJECT`,`VERTEXAI_LOCATION`,`AWS_ACCESS_KEY_ID`,`AWS_SECRET_ACCESS_KEY`,`AWS_REGION`,`AWS_PROFILE`,`AWS_BEARER_TOKEN_BEDROCK`,`AZURE_OPENAI_API_ENDPOINT`,`AZURE_OPENAI_API_KEY`,`AZURE_OPENAI_API_VERSION`,`MOONSHOT_API_KEY`,`XAI_API_KEY`. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** Hyper subscription OAuth and provider keys through picker; Bedrock API token or AWS chain/SSO; config `aws_auth_refresh` command runs on credential error then retries in place, avoids duplicate messages; Vertex application-default gcloud. `crush login/logout` registered. Exact credentials-on-disk format and offline expired-session probe unknown. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go

## 16. Running many instances

- **[docs]** Shared backend workspaces keyed canonical cwd share session list/history/permission queue/LSP/MCP. Each invocation fresh session unless resume. First creator fixes debug/yolo; later mismatched flags logged, don't change workspace. Workspace lives while any SSE stream open, small post-create grace; last disconnect tears down. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
- **[docs]** Auto-start singleflight exclusive flock `<XDG_CACHE_HOME>/crush/server-<safeHost>/start.lock`; reprobe200ms after lock, readiness100ms attempts over default10sec; lock failure falls back unsynchronized. Per-host server logs/stdout/stderr. Client env `os.Environ()` sent in workspace request, so backend receives launch environment and secrets; ensure trusted server. https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go
- Memory/instance, DB contention guarantees local-mode concurrent same data-dir, account quotas unknown. Shared backend knowingly shares state; separate cwd/data roots when isolation intended.

## 17. Anything else a program driving this harness should know

- **[docs]** Provider auto-updates mutate available models; `crush update-providers [URL|LOCAL_FILE|embedded]` explicit refresh. Metrics pseudonymous metadata not prompt/response, optout env above. Normal startup creates data directory0700 and `.gitignore` `*`, registers project and opens DB: not read-only. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md https://raw.githubusercontent.com/charmbracelet/crush/main/internal/cmd/root.go
- **[docs]** Author attribution default `Assisted-by: Crush:[ModelID]` plus generated-with; `option attribution-trailer-style assisted-by|co-authored-by|none`, `option attribution-generated-with true|false`. Configuration executable shell and permission tiers need review independently; default interface being Go TUI says nothing about safe unattended permission policy. https://raw.githubusercontent.com/charmbracelet/crush/main/README.md
