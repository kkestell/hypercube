# Amp

## 1. Overview

- **[verified]** Research 2026-10-30; `amp` not on PATH, no install/config changes/model calls. Runtime version unknown; current official docs checked, not a pinned release.
- **[docs]** Amp coding agent/development environment at ampcode.com, proprietary service; installs through `https://ampcode.com/install.sh`. Current docs explicitly emphasize no backwards compatibility and automatic updates. Exact legal vendor entity/license, release interval and CLI user count unknown. https://ampcode.com/docs https://ampcode.com/docs/cli

## 2. Interactive terminal interface

- **[docs]** Interactive TUI default; piped input becomes first interactive message unless `--execute/-x`. Clipboard paste Ctrl+V; command palette Ctrl+O; Windows supported via WSL, WezTerm/Alacritty recommended over Windows Terminal for clipboard. https://ampcode.com/docs/cli
- **[docs]** `amp.notifications.enabled` defaults true: completion/blocked audio, terminal BEL over SSH or `AMP_FORCE_BEL`. `amp.terminal.copyOnSelect`, `amp.terminal.detailsExpandedByDefault`, `amp.keymap` customize UI; Alt+T expands details. https://ampcode.com/docs/cli/settings
- Alternate-screen/title OSC/OSC 9/9;4/7501, mouse protocol, resize behavior: unknown.

## 3. Launch and configuration

- **[docs]** `--settings-file PATH` replaces user settings location; user `~/.config/amp/settings.json|jsonc`, workspace nearest `.amp/settings.json|jsonc` searched upward to repository root (cwd only outside Git), workspace overrides user. Exception: user keymap entries override workspace. `amp config edit [--workspace]` mutates files and was not run. https://ampcode.com/docs/cli/settings
- **[docs]** Process flags established in checked docs: `--execute/-x`, `--stream-json`, `--stream-json-input`, `--stream-json-thinking`, `--executor local|orb|runner:ID`, `--runner-dir ABSOLUTE_PATH`, `--settings-file`, `--no-tui`, `--runner-id`, `--amp-env`, `--project`. `AMP_API_KEY` overrides saved auth, `AMP_SKIP_UPDATE_CHECK=1` overrides `amp.updates.mode` (`auto|warn|disabled`), `AMP_DISABLE_AMP_THREAD_TRAILER=1`, `AMP_FORCE_BEL`, proxies `HTTP_PROXY`,`HTTPS_PROXY`,`NODE_EXTRA_CA_CERTS`. https://ampcode.com/docs/cli https://ampcode.com/docs/cli/settings https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/plugin-api
- **[docs]** Relevant keys: `amp.mcpServers`, `amp.mcpPermissions`, `amp.tools.disable`, `amp.skills.path`, `amp.skills.disableClaudeCodeSkills`, `amp.skills.disableGlobalAgentsSkills`, `amp.notifications.enabled`, `amp.showCosts`, `amp.defaultVisibility`, `amp.remoteThreadCreation.enabled`, `amp.runner.autoUpdate.enabled`, `amp.runner.env.enabled`, `amp.thread.autoArchiveOnQuit`, Git attribution controls. https://ampcode.com/docs/cli/settings
- **[docs]** Managed local config macOS `/Library/Application Support/ampcode/managed-settings.json`, Linux `/etc/ampcode/managed-settings.json`, Windows `%ProgramData%\ampcode\managed-settings.json`; local managed > workspace-admin document > normal local settings. Scalars override, arrays combine managed-first, objects merge managed winning; admin document sync about 10 minutes/startup. https://ampcode.com/docs/cli/settings
- Complete flags/environment inventory and CLI-only explicit plugin-path switch unknown (binary unavailable). Settings redirection alone does not promise suppression of project/plugin/managed files.

## 4. Models and reasoning

- **[docs]** Current built-in agent modes `low`,`medium`,`high`,`ultra`; deprecated `rush` maps low, `smart`/`deep` map medium for newly spawned threads. Plugins `createAgent({model:'provider/model',reasoningEffort,...})`, effort `none|minimal|low|medium|high|xhigh|max`; `extends` inherits built-in prompt/model/tools. `amp plugins show-agent-options --json` discovers public models and builtin tool names. https://ampcode.com/docs/plugin-api
- Generic CLI model/provider flags, unknown-model handling and exact main-thread model pinning syntax: unknown in checked pages. Do not assume old smart/deep mode documents describe current modes.

## 5. Conversations

- **[docs]** Threads use `T-UUID` IDs. Streaming init/user/assistant/result carry `session_id`. `amp threads continue --execute TEXT --stream-json` resumes an existing thread; caller-selected new ID and exact list/fork/export/delete command flags unknown. https://ampcode.com/docs/cli/streaming-json
- **[docs]** Plugin thread APIs include `amp.threads.get(threadID)`, `appendUserMessage(message,{steer:true})`, `ctx.thread.append`, agent `createThread`, `waitForResponse` and cancellation support. `compactionThresholdTokens` sets custom-agent positive token trigger, default configured percentage 90%. https://ampcode.com/docs/plugin-api
- Exact local storage paths, export formats, file locking and multiple processes editing one thread: unknown. Threads are available across clients/runners/orbs, not necessarily local-file-only. **[docs]** https://ampcode.com/docs/cli

## 6. Transcript and structured output

- **[docs]** `--execute --stream-json`: one JSON per line. Types `system` subtype `init|subagent_finished|error_max_turns|error_during_execution`, `user`, `assistant`, `result` subtype `success|error_max_turns|error_during_execution`. Init `{cwd,session_id,tools,mcp_servers:[{name,status}],agent_mode?,subagents:[{tool_use_id,description}]}`. MCP statuses `awaiting-approval|authenticating|connecting|reconnecting|connected|denied|failed|blocked-by-registry`. https://ampcode.com/docs/cli/streaming-json
- **[docs]** Assistant `{message:{type:'message',role:'assistant',content,stop_reason,usage?},parent_tool_use_id,session_id}`; content text `{type,text}` or tool_use `{type,id,name,input}`; opt-in thinking `{type:'thinking',thinking}` / `{type:'redacted_thinking',data}` via `--stream-json-thinking` breaks strict Claude compatibility. Stop reasons `end_turn|max_tokens|stop_sequence|tool_use|pause_turn|refusal|null`. User content text or tool_result `{tool_use_id,content,is_error}`; optional `subagent_tool_use_id`. https://ampcode.com/docs/cli/streaming-json
- **[docs]** Final result `{duration_ms,duration_api_ms?,is_error,num_turns,result|error,session_id,usage?,permission_denials?}`. Usage fields `input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens,cache_creation:{ephemeral_5m_input_tokens,ephemeral_1h_input_tokens},max_tokens,service_tier`. https://ampcode.com/docs/cli/streaming-json
- Live transcript file format, stable diff/todo projection: unknown; stream tool events expose tools actually invoked.

## 7. Hooks and events

- **[docs]** Plugin callbacks `amp.on('session.start'|'tool.call'|'tool.result'|'agent.start'|'agent.end', handler)` demonstrated; type reference is authoritative for any additional events. These are API callbacks, not claimed complete shell-hook compatibility. `session.start` has `event.thread.id`; tool.call `event.tool` and call details interpreted by `amp.helpers.shellCommandFromToolCall/filesModifiedByToolCall`; tool.result `event.tool,event.status`; agent.start/end `event.message`, end `event.messages`. https://ampcode.com/docs/plugin-api
- **[docs]** tool.call response can `{action:'allow'}` or `{action:'reject-and-continue',message}`. agent.start can inject `{message:{content,display:true}}`; agent.end can `{action:'continue',userMessage}` for automatic follow-up. Order of same-event plugin handlers is undefined. Plugin examples guard repeated continuation themselves; general timeout/output/loop caps unknown. https://ampcode.com/docs/plugin-api
- **[docs]** `onDispose` cleanup runs unload/reload/graceful shutdown concurrently with approximately 3 seconds shared budget; not crash/SIGKILL. Webhook callbacks have 30 seconds, retry exponential 5 sec–5 min, discarded after one hour; at-least-once use `event.id` idempotency. These webhook limits are not general hook limits. https://ampcode.com/docs/plugin-api

## 8. Plugins and extensions

- **[docs]** Bun-executed JS/TS plugins `.amp/plugins/`, `~/.config/amp/plugins/`; default export receives PluginAPI. Long-lived processes may handle concurrent threads. Reload via command palette `plugins: reload`. https://ampcode.com/docs/plugin-api
- **[docs]** Public API inventory: `logger`, `system`, observable `configuration`, `$` shell, `helpers`, `ui`, `on`, `registerCommand`, `registerTool`, `registerSkill`, `registerLinkPattern`, `onDispose`, `createWebhook`, `ai`, `attachments`, `createAgent`, `getBuiltinAgent`, `registerAgentMode`, observable `activeThread`, `threads`, optional `experimental`. UI notify/input/select/confirm (confirm supports editable form fields), system open and metadata; AI ask/generate structured/text helpers. No demonstrated custom provider registration; model routing can select provider/model. https://ampcode.com/docs/plugin-api
- **[docs]** Custom tools `{name,title?,transcriptGroup?,description,inputSchema,execute}`; skills explicitly register `skills/...` directory (not auto-scanned), absolute generated skills confined to plugin cache directory. Custom mode registration requires matching `// @amp-agent-mode {key,label}` metadata, unique max-24-character labels/keys. `createAgent` tools supports all/list or include/add/exclude globs; explicit lists replace delegation defaults. https://ampcode.com/docs/plugin-api
- **[docs]** Agent APIs `definition`, `createThread(options)`, `run(message,options)`; timeout default ten minutes rejects caller but leaves child finishing turn; aborting enclosing plugin tool cancels child. Threads can target local/orb/runner, set parent, visibility and multiplayer. https://ampcode.com/docs/plugin-api
- Process-scoped plugin loading without writing shared/user/project discovery dirs: unknown.

## 9. Tools, permissions, and approvals

- **[docs]** `amp.tools.disable` supports bare/builtin-qualified names and globs. `amp.mcpPermissions` first-match rules `matches:{command,args}` or `{url}`, `action:allow|reject`; default allow. Block-all requires command and URL rules. https://ampcode.com/docs/cli/settings
- **[docs]** Stream init enumerates effective tool set, including example Bash/read/edit/create/glob/grep/Task/todo tools. Plugin tool.call can implement permission controls/UI confirms. https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/plugin-api
- Exact general CLI approval modes/sandbox flags and headless external approval protocol: unknown. MCP server acceptance and tool execution permissions are different concerns.

## 10. Subagents and delegation

- **[docs]** Task `mode:'async'` launches background subagents. `subagent_finished` `{tool_use_id,status:completed|failed|cancelled,session_id}`; final summary can start another parent turn. Background subagents outlive initiating process; resume init lists remaining ones. Final `result`, not assistant end_turn, signals entire run complete. https://ampcode.com/docs/cli/streaming-json
- **[docs]** Plugin agents can exclude tools; include/exclude names `Task`,`create_thread`,`list_agent_modes`,`get_thread_status`,`send_thread_message`,`wait_for_threads` control respective delegation/coordination surfaces. Task does not accept per-call agent mode; persistent child-thread tools do. Global disable-all-native-subagents flag unknown. https://ampcode.com/docs/plugin-api

## 11. MCP

- **[docs]** `amp.mcpServers` config, command/args or URL permission matching; skill bundles may start MCP servers when loaded. Names `mcp__<server>__<tool>`. Plugin agents normally reach deferred MCP via `tool_search` and `code_exec`, not each tool individually in prompt; excluding both sends selected tools directly. https://ampcode.com/docs/cli/settings https://ampcode.com/docs/plugin-api
- Complete transport/resource/prompt/elicitation matrix and unsolicited MCP-to-thread push unknown. Plugin webhook + appendUserMessage is a separate supported push surface.

## 12. Control from outside

- **[docs]** `--execute --stream-json --stream-json-input` is bidirectional persistent process protocol. Input NDJSON `{type:'user',steer?:boolean,message:{role:'user',content:[{type:'text',text}|image]}}`. Image source base64 with JPEG/PNG/GIF/WebP, optional `source_path`; output omits user images. Busy input queues, `steer:true` takes next interruption point; echo emitted when turn starts, **not receipt acknowledgment**. Process exits only once assistant/background work done and stdin closed. https://ampcode.com/docs/cli/streaming-json
- **[docs]** `amp --no-tui --runner-id ID` runs connected runner; `--executor runner:ID --runner-dir PATH` opens TUI for runner thread; orb/local also supported. `amp.remoteThreadCreation.enabled` allows web-created threads in local TUI. Plugin APIs can send cross-thread messages and cancel/await. https://ampcode.com/docs/cli https://ampcode.com/docs/cli/settings https://ampcode.com/docs/plugin-api
- Public ACP or raw local JSON-RPC endpoint, stdout request to interrupt, precise signal semantics: unknown.

## 13. Status from outside

- **[docs]** Stream init describes tools/MCP/subagents; assistant end_turn means turn boundary only; result means run over. System error and result error report failures; plugin tool/agent lifecycle and observable threads support richer monitors. https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/plugin-api
- Waiting-for-human state JSON protocol and auth-independent live-status endpoint unknown.

## 14. Usage, cost, and limits

- **[docs]** Token/cache accounting in streamed assistant/final result usage; optional permission_denials and durations. `amp.showCosts` default true (workspace admins can hide). Custom agent compaction default 90% with positive absolute override. https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/cli/settings https://ampcode.com/docs/plugin-api
- Programmatic dollar ledger/rate-limit endpoint and total process quota controls unknown.

## 15. Authentication

- **[docs]** `amp login`, `amp account list`, `amp account switch EMAIL_OR_ID`, `amp logout [EMAIL_OR_ID|--all]`; accounts separated per Amp server. Process pins initial account; switch only affects future processes. `AMP_API_KEY` takes precedence until unset, even after logout. https://ampcode.com/docs/cli
- Exact credential disk location, no-model-call expiry check guarantees unknown; account listing only proves saved account existence.

## 16. Running many instances

- **[docs]** Plugins may serve multiple threads concurrently, so per-thread state must be keyed. Running account pinning and persistent subagents mean process teardown is not equivalent to all work teardown. Runner auto-updates at idle, at most every 12 hours, preserving flags; disable with settings/`AMP_SKIP_UPDATE_CHECK`. https://ampcode.com/docs/plugin-api https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/cli/settings
- Shared file locks, memory/instance, conflict guarantees, quotas and safe same-thread simultaneous control unknown.

## 17. Anything else a program driving this harness should know

- **[docs]** Stdout user echo lags input receipt; final result is sole whole-run terminator; descendants survive process exit. A correct adapter needs all three facts. Auto-update defaults and API evolution warrant recording installed version for every run. https://ampcode.com/docs/cli/streaming-json https://ampcode.com/docs/cli
