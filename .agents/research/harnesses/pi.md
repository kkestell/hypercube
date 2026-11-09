# Pi

## 1. Overview

- **[verified]** `pi --version` returned `1.4.0`; `pi --help` and `pi auth --help` were inspected. No session/model requests were made.
- **[docs]** Earendil Works' Pi (formerly `badlogic/pi-mono`), MIT; current package `@earendil-works/pi-coding-agent`, Node >=22.19. Install with the pi.dev installer or `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`. The installer pins dependencies; ordinary npm does not. Source: https://github.com/earendil-works/pi/tree/main/packages/coding-agent
- Release cadence and numeric adoption: unknown; unpinned `main` documentation is not a guarantee about every installed 1.4.0 detail.

## 2. Interactive terminal interface

- **[help]** `--tui-mode fullscreen|regular`; fullscreen is default, regular preserves the conventional terminal interaction style. `--use-theme`, `--theme`, `--no-themes` customize display.
- **[docs]** Settings include `tuiMode`, `fullscreenWheelScrollLines`, `showHardwareCursor`, `terminal.showImages`, `terminal.showTerminalProgress` (default false), `terminal.hyperlinks`, `terminal.images` (`kitty|iterm2|auto|false`), `terminal.trueColor`. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/settings.md
- **[docs]** Terminal source emits OSC 0 title (`ESC ] 0 ; title BEL`), optional indeterminate progress `ESC ] 9;4;3 BEL` and clear `ESC ] 9;4;0 BEL`, with one-second keepalive. OSC 7501 support is queried; `PI_PROGRAM_STATUS=1` forces support. It negotiates Kitty keyboard flags 7, bracketed paste, device attributes, listens for stdout resize, and refreshes POSIX dimensions with SIGWINCH. Source: https://github.com/earendil-works/pi/blob/main/packages/tui/src/terminal.ts
- **[docs]** OSC7501 query `ESC ] 7501;? ST`; supporting terminal echoes body; report `ESC ] 7501;state=STATE:app=APP:kind=KIND:msg=BASE64 ST`. States idle|working|blocked|done|error|clear; kind permission|question|auth only blocked; app `[A-Za-z0-9_.+-]{1,32}`; message controls sanitized and UTF-8 truncated2048 bytes before base64 (encoded under2732 bytes); root record only. Source: https://github.com/earendil-works/pi/blob/main/packages/tui/src/program-status.ts
- **[docs]** Apple Terminal can send ESC+CR for Shift+Enter; Pi has native-modifier normalization for that case. Source: same terminal source URL above. Exact notification OSC 9 policy, all fullscreen sequences and mouse protocol details: unknown.

## 3. Launch and configuration

- **[help]** Process flags: `--provider`, `--model`, `--api-key`, `--system-prompt`, repeatable `--append-system-prompt`, `--mode text|json|rpc`, `--print`, `--thinking`, `--models`, `--name`; `@files` and positional messages. `--` ends argument parsing.
- **[help]** Resource isolation: repeatable `-e/--extension`, `--no-extensions` (explicit `-e` still loads), `--skill`, `--no-skills`, `--prompt-template`, `--no-prompt-templates`, `--theme`, `--no-themes`, `--no-context-files`, `--no-mcp`. `--approve` trusts project files for this run; `--no-approve` ignores them. Extensions may register extra flags.
- **[help]** Roots: `PI_CODING_AGENT_DIR` defaults `~/.pi/agent`; `PI_CODING_AGENT_SESSION_DIR`, `--session-dir`, `--session`, `--session-id`, `--no-session`. `PI_PACKAGE_DIR` overrides package directory. `--offline` / `PI_OFFLINE=1` disables startup network operations; this is not a blanket provider-network prohibition.
- **[docs]** Global `<agentDir>/settings.json` merges with project `.pi/settings.json`; project values override global, nested objects merge. Settings cover models, tools, sessions, compaction, retries, shell, display, packages/extensions/skills/prompts/themes. No general CLI inline-settings JSON override was established; SDK `SettingsManager` permits host-controlled settings. Sources: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/settings.md and https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md

## 4. Models and reasoning

- **[help]** `--model` accepts pattern/ID, `provider/id`, and `:thinking` suffix; `--provider` requires `--model`. `--models` supplies comma-separated globs/fuzzy patterns for cycling. `--list-models [search]` is the listing interface. Thinking: `off|minimal|low|medium|high|xhigh|max`.
- **[docs]** RPC `get_available_models`, `set_model {provider,modelId}`, `cycle_model`, `set_thinking_level`, `cycle_thinking_level`, `get_available_thinking_levels`; an invalid RPC model returns `success:false,error:"Model not found: ..."`. Sources: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md and https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md
- **[docs]** Extensions register providers and model inventories, auth and streaming behavior through `registerProvider`; `refreshModels` supports changing inventories. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md

## 5. Conversations

- **[help]** `--session-id <id>` opens exact project ID or creates it; `--session <path|partial UUID>` selects existing session; `--continue`, `--resume`, `--fork <path|id>`, `--session-dir`, `--no-session`, `--name`. `--export <file>` exports HTML and exits.
- **[docs]** Default files are `~/.pi/agent/sessions/<encoded-cwd>/*.jsonl`. Session format is an append-only tree: header plus entries with `id`, `parentId`, `timestamp`; branching can retain multiple histories in one file. Types include message, model/thinking changes, compaction, branch summary, custom/custom-message, label, session info. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/session-format.md
- **[docs]** RPC includes `new_session`, `switch_session`, `fork`, `clone`, `get_fork_messages`, `get_entries`, `get_tree`, `set_session_name`, `export_html`. SDK `SessionManager` provides listing and storage control. Sources: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md and https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md
- **[docs]** `compact`, `set_auto_compaction`; `session_before_compact` can replace/cancel summarization. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md
- Session deletion API, same-ID simultaneous-writer locking guarantees: unknown. Use separate IDs/files, not concurrent writers to one transcript.

## 6. Transcript and structured output

- **[help]** `--mode json` emits structured output; `--mode rpc` is bidirectional; `--print` exits after processing. `--export` writes HTML.
- **[docs]** JSONL events: `agent_start`, `agent_end {messages,willRetry}`, `agent_settled {aborted}`, `turn_start`, `turn_end {message,toolResults}`, `message_start/end {message}`, `message_update {usage,assistantMessageEvent}`. Stream updates include `text_*`, `thinking_*`, `toolcall_*` with `contentIndex`, deltas and final content. Trust `message_end` as final authoritative message. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/json.md
- **[docs]** Tools emit `tool_execution_start {toolCallId,toolName,args}`, `tool_execution_update {partialResult,...}`, `tool_execution_end {result,isError,durationMs,...}`. Queues emit `queue_update {steering,followUp}`. Custom entries, session naming, thinking changes, compaction and retry events are also streamed. Source: same JSON reference.
- **[docs]** Transcript entries can be read during runtime via RPC/SDK; `get_messages` and `get_entries` differ (projected messages versus persistence entries). No built-in plan mode/subagent system is promised; extensions can implement plans/todos/diffs. Sources: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md and https://github.com/earendil-works/pi/tree/main/packages/coding-agent

## 7. Hooks and events

- **[docs]** Hooks are extension `pi.on(event,handler)` functions, not a separate shell-hook settings schema; load one per process with `-e`. Async factories are awaited before startup and resource discovery. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md
- **[docs]** Lifecycle/resource events: `project_trust`, `resources_discover`, `mcp_servers_change`, `session_start`, `session_info_changed`, `session_before_switch`, `session_before_fork`, `session_before_compact`, `session_compact`, `session_compact_failed`, `session_before_tree`, `session_tree`, `session_shutdown`. `session_start.reason`: `startup|reload|new|resume|fork`; shutdown reason: `quit|reload|new|resume|fork`. Before-events can cancel/customize corresponding operations. Source: same extension reference.
- **[docs]** Agent/input events: `input`, `before_agent_start`, `agent_start`, `agent_end`, `agent_before_settle`, `agent_settled`, `ui_prompt_start/end`, `turn_start/end`, `message_start/update/end`; tool events: `tool_call`, `tool_result`, `tool_execution_start/update/end`, `user_bash`; request events: `context`, `context_with_system`, `before_provider_headers`, `before_provider_request`, `after_provider_response`, `provider_stream_event`, `cache_warming_decision`; selection events: `model_select`, `thinking_level_select`. Source: same extension reference.
- **[docs]** `tool_call` mutates `event.input`; returns `{block:true,reason?,terminate?}`. Sibling preflights are sequential, execution can be parallel; handlers must not assume sibling results already exist. Unknown-tool/invalid-input and blocked calls still produce `tool_execution_end` but can skip `tool_call`/`tool_result`. Source: same extension reference.
- **[docs]** `input` may handle/transform input; `before_agent_start` changes system-prompt inputs and injects messages; context hooks replace outgoing context; provider hooks change headers/body/results; tool-result hooks rewrite results. Handlers compose in load order; return and mutation semantics are event-specific, not uniform. Source: same extension reference.
- Generic hook timeout/output-size/re-entry limits: unknown. Extension callbacks execute arbitrary trusted code; do not assume shell-hook isolation or a global loop cap.

## 8. Plugins and extensions

- **[docs]** TypeScript/JavaScript extension factory receives `ExtensionAPI`; automatic locations include `~/.pi/agent/extensions/` and `.pi/extensions/`; packages can bundle extensions, skills, prompts and themes. `-e` loads explicit files without installation; `--no-extensions` disables discovery/builtins but not explicit paths. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md
- **[docs]** Registration/control surface includes `registerTool`, `registerCommand`, `registerShortcut`, `registerFlag`, `registerProvider`, `registerMessageRenderer`, `registerEntryRenderer`, `registerMarkdownTransformer`, `on`; `sendMessage`, `sendUserMessage`, `appendEntry`, `setSessionName`, `setLabel`, `getCommands`, tool activation/model selection and execution helpers. Source: same extension reference.
- **[docs]** Remaining exact methods: getSessionName, exec, getActiveTools, getAllTools, setActiveTools, setModel, getThinkingLevel, setThinkingLevel, events, unregisterProvider, registerVirtualModel/unregisterVirtualModel, registerMcpServer/unregisterMcpServer/getMcpServers, registerToolRenderer, getSettings. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md
- **[docs]** Context exposes session manager, model registry/current model/thinking/scoped models, cwd, trust, signal, idle/pending-state, abort/shutdown, context usage/compaction/system prompt; command context adds wait-for-idle, new/switch/fork/tree navigation/reload. UI can add status, widgets, headers/footers, editors, dialogs, custom components and overlays. Source: same extension reference.
- **[docs]** SDK `createAgentSession` embeds runtime; host may supply resource loader, model registry, auth/settings/session managers, tools and custom tools. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md

## 9. Tools, permissions, and approvals

- **[help]** Builtins: `read,bash,powershell,edit,write,grep,find,ls`; grep/find/ls off by default. `--no-tools`, `--no-builtin-tools`, `--tools` allowlist/patterns or `+/-` modifications, `--exclude-tools` denylist; these distinguish MCP and extension tools.
- **[docs]** Pi is not a built-in per-command approval/sandbox security boundary; extensions can intercept `tool_call` to request UI confirmation or block. Project trust is separate from execution sandboxing. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md
- **[docs]** RPC extension UI request/response records carry IDs for select/confirm/input/editor interactions; `extension_ui_response` answers that ID, not an ordinary command. Unsupported full custom TUI operations require a richer host/SDK UI. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md

## 10. Subagents and delegation

- **[docs]** Pi intentionally omits native subagents and plan mode; extensions/packages implement them. Source: https://github.com/earendil-works/pi/tree/main/packages/coding-agent
- **[help]** Restrict extension-supplied delegation through `--no-extensions`, selective `-e`, and tool allow/deny patterns. Built-in background agent teams: not established.

## 11. MCP

- **[help]** Current 1.4.0 has `pi mcp`, `--no-mcp`, MCP tool filtering; this contradicts older Pi descriptions claiming no MCP support.
- **[docs]** Built-in MCP supports stdio and Streamable HTTP, **rejects legacy SSE**, supports OAuth, tools and resources. Config uses `mcpServers` in agent/project `mcp.json`; project entries replace same-name user entries after trust. MCP names use `mcp__<server>__<tool>`. Extensions can `registerMcpServer` for this session; an extension replacing `/mcp` replaces built-in MCP entirely. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md
- MCP prompt discovery and server-initiated elicitation support: unknown.
- Unsolicited MCP notification causing an autonomous new user turn: unknown; do not equate notifications/catalog updates with prompt submission.

## 12. Control from outside

- **[docs]** `pi --mode rpc` is persistent JSONL over stdin/stdout, diagnostics stderr. Each command has optional string `id`; response `{type:"response",command,success,data?,error?,id?}`; responses can arrive out of order. Split strictly on LF, not Unicode U+2028/U+2029; read stdout continuously and honor both directions' backpressure. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md
- **[docs]** Commands inventory: `prompt`, `steer`, `follow_up`, `abort`, `clear_queue`, `new_session`, `get_state`, `get_messages`, `set_model`, `cycle_model`, `get_available_models`, `set_thinking_level`, `cycle_thinking_level`, `get_available_thinking_levels`, `set_steering_mode`, `set_follow_up_mode`, `compact`, `set_auto_compaction`, `set_auto_retry`, `abort_retry`, `bash`, `abort_bash`, `get_session_stats`, `export_html`, `switch_session`, `fork`, `clone`, `get_fork_messages`, `get_entries`, `get_tree`, `get_last_assistant_text`, `set_session_name`, `get_commands`. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md
- **[docs]** `prompt {message,images?,streamingBehavior?}` returns disposition `started|queued|handled`; while streaming require `streamingBehavior:"steer"|"followUp"`. Steering lands after current tools before next model call; follow-up waits until agent stops. Extension slash commands can execute immediately while streaming. `abort` waits until idle; EOF gracefully disposes runtime. Source: same command reference.
- **[docs]** SDK and exported `RpcClient` offer typed integrations; no built-in network daemon/TUI attach-to-RPC process was established. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md

## 13. Status from outside

- **[docs]** Use `get_state`, run/message/tool/retry/compaction/queue events. **`agent_end` is not final idle**: retries/recovery/queued messages may continue; use `agent_settled`. Subscribe before submitting to avoid missing fast completion; a `handled` prompt need not produce a run. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md
- **[docs]** get_state.data exact fields: model? (full model object), thinkingLevel,isStreaming,isCompacting,steeringMode,followUpMode,sessionFile,sessionId,sessionName?,autoCompactionEnabled,messageCount,pendingMessageCount. Modes all|one-at-a-time select queue delivery. new_session `{parentSession?:path}` returns `{cancelled:boolean}`; extension veto still success:true. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md
- **[docs]** `extension_ui_request` denotes waiting for host input; tool errors carry `isError`, provider stream errors are message events. OSC 7501 is terminal-facing state, not a replacement for RPC lifecycle. Sources: RPC reference above and terminal source cited in section 2.

## 14. Usage, cost, and limits

- **[docs]** `get_session_stats`, message usage, context usage via extension `ctx.getContextUsage()`, JSON `message_update.usage`/final messages expose accounting; model prices drive estimates, not guaranteed billing. Sources: RPC commands, JSON, extensions references above.
- **[docs]** Stats fields sessionFile/sessionId,userMessages,assistantMessages,toolCalls,toolResults,totalMessages,tokens `{input,output,cacheRead,cacheWrite,total}`,cost,contextUsage? `{tokens,contextWindow,percent}`. Full-session counters include tools and compaction/branch summaries; contextUsage omitted without model/window and tokens/percent null after compaction until fresh response. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md
- **[docs]** Retries configurable with `retry.enabled/maxRetries/baseDelayMs/maxAgentDelayMs` and provider retry/timeout keys. `cacheWarming` can make transient model calls and add usage outside visible conversation (default `streaming`; optional `idle`). Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/settings.md
- Hard dollar quota, global multi-process rate limiter and universal token cap: unknown.

## 15. Authentication

- **[help]** `--api-key` or many provider env vars (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_OAUTH_TOKEN`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY`, `OPENCODE_API_KEY`, cloud credentials, etc.). `pi auth check --provider X --json --no-refresh` checks readiness without exposing/refreshing credentials; omit `--credentials` to avoid secret output.
- **[help]** `auth print-api-key` and `auth print-bearer-token` intentionally print secrets; bearer-token output may refresh expired OAuth. Do not use them as ordinary status probes.
- **[docs]** AuthStorage uses `<agentDir>/auth.json`; SDK can provide an alternate/in-memory auth store. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md
- Detecting provider-revoked credentials without contacting provider: unknown; local readiness is not proof of remote validity.

## 16. Running many instances

- **[help]** Independent RPC child per worker; isolate with `PI_CODING_AGENT_DIR`, `--session-dir` and distinct sessions. Working directories remain ordinary shared files unless caller supplies worktrees.
- **[docs]** Global settings/auth/resources are shared by default; cache-warming and provider retries can add aggregate load. A slow stdout consumer stalls RPC. Sources: settings and RPC references above.
- Per-instance RSS, credential-refresh locking details, maximum process count, concurrent same-session mutation safety: unknown; not benchmarked under no-model-call rules.

## 17. Anything else a program driving this harness should know

- **[docs]** Treat acceptance, low-level run completion and fully settled idle as three distinct boundaries. Handle malformed JSON responses, async provider failures, child exits, EOF shutdown and your own deadlines. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md
- **[help]** `--offline --no-extensions --no-context-files --no-mcp` is useful for reducing startup side effects; explicit `-e` still loads and can execute code. None of the research invoked a prompt or RPC runtime.
- **[docs]** Compaction events start `{reason:manual|threshold|overflow}`; end `{reason,result?:{summary,firstKeptEntryId,tokensBefore,estimatedTokensAfter,usage,details},aborted,willRetry,errorMessage?}`. Aborted→result absent/aborted true; failure→result absent/aborted false/errorMessage; overflow success can willRetry true. auto_retry_start `{attempt,maxAttempts,delayMs,errorMessage}`, auto_retry_end `{success,attempt,finalError?}`. Summary retry scheduled uses same attempt/max/delay/error; attempt_start `{source:compaction|branchSummary,reason?:manual|threshold|overflow}`, finished event follows. Source: https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/json.md
- **[docs]** RPC bash_execution_update `{id?,delta}` streams all output even when final bash response truncates; extension_error `{extensionPath,event,error}` reports callback throws. Source: same JSON reference.
