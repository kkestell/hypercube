# Cursor CLI

## 1. Overview

- **[verified]** Research 2026-10-30: neither `cursor-agent` nor `agent` is on PATH. No installed version, terminal capture, install or model call.
- **[docs]** Anysphere/Cursor terminal agent; official current executable examples use `agent`. Install instructions and proprietary product are at https://cursor.com/docs/cli/overview . Exact CLI build/version and license terms: unknown. Docs snapshot checked, not a versioned release.
- CLI-only adoption and release cadence: unknown; Cursor editor adoption is not a measurement of its CLI.

## 2. Interactive terminal interface

- **[docs]** Default invocation starts interactive agent mode; `--print` disables interactive presentation for scripts. https://cursor.com/docs/cli/reference/parameters
- Exact alternate-screen behavior, title OSC, OSC 9/9;4/7501, bell, mouse, resize and terminal-specific bugs: unknown; not captured locally.

## 3. Launch and configuration

- **[docs]** Global launch options: `-v/--version`, `--api-key KEY`, repeat `-H/--header 'Name: Value'`, `-p/--print`, `--output-format text|json|stream-json`, `--stream-partial-output`, `--resume [chatId]`, `--continue`, `--model MODEL`, `--mode plan|ask`, `--plan`, `--list-models`, `-f/--force`, `--yolo`, `--sandbox enabled|disabled`, `--approve-mcps`, `--trust` (headless only), `--workspace PATH`, repeat `--plugin-dir PATH`, `-w/--worktree [name]`, `--worktree-base BRANCH`, `--skip-worktree-setup`, help. Initial prompt is positional. https://cursor.com/docs/cli/reference/parameters
- **[docs]** `CURSOR_API_KEY` supplies auth without writing files. MCP locations `.cursor/mcp.json` and `~/.cursor/mcp.json`. https://cursor.com/docs/cli/reference/parameters
- Config-root override, full CLI config keys/layers/precedence and hook-isolation environment: unknown in checked sources. A plugin-dir is additive, not evidence that user plugins/hooks stop loading.

## 4. Models and reasoning

- **[docs]** `--model MODEL`, `--list-models` or `agent models` (account-specific). `--mode plan|ask` controls operation, not reasoning effort. https://cursor.com/docs/cli/reference/parameters
- Reasoning/variant CLI syntax and unknown-model handling: unknown. Hooks can report `model_id` and `model_params:[{id,value}]`, but that does not establish CLI flags. **[docs]** https://cursor.com/docs/agent/hooks

## 5. Conversations

- **[docs]** `agent create-chat` creates empty chat and returns its ID. `--resume [chatId]`; `--continue` aliases `--resume=-1`; `agent resume` resumes latest; `agent ls` opens history picker. https://cursor.com/docs/cli/reference/parameters
- **[docs]** Stream init and final results carry `session_id` UUID. https://cursor.com/docs/cli/reference/output-format
- Caller-chosen ID, fork/export/delete/compaction CLI, exact session storage and concurrent same-session ownership rules: unknown.

## 6. Transcript and structured output

- **[docs]** Print mode explicit or inferred from non-TTY stdout/piped stdin. `json` emits one final success object; failures emit stderr and nonzero exit with **no guaranteed JSON object**. `stream-json` is NDJSON and can end without terminal event on failure. https://cursor.com/docs/cli/reference/output-format
- **[docs]** Events: `system/init` fields `apiKeySource`, `cwd`, `session_id`, `model`, `permissionMode`; `user`/`assistant` fields `message:{role,content:[{type:'text',text}]}`, `session_id`; `tool_call` subtype `started|completed`, `call_id`, `tool_call`, `session_id`; final `result/success` fields `duration_ms`, `duration_api_ms`, `is_error:false`, `result`, `session_id`, optional `request_id`. https://cursor.com/docs/cli/reference/output-format
- **[docs]** Tool payloads include `readToolCall.args.path`, `.result.success.{content,isEmpty,exceededLimit,totalLines,totalChars}`; `writeToolCall.args.{path,fileText,toolCallId}`, success `{path,linesCreated,fileSize}`; others can use `function:{name,arguments}`. Thinking is suppressed in print mode. https://cursor.com/docs/cli/reference/output-format
- **[docs]** Partial-stream duplicate trap: only append assistant events with `timestamp_ms` present and `model_call_id` absent. Timestamp + model_call_id is duplicate pre-tool flush; neither is duplicate final flush. Ignore unknown future fields. https://cursor.com/docs/cli/reference/output-format
- **[docs]** Hooks expose `transcript_path:string|null`, allowing log discovery when enabled; exact CLI transcript serialization/storage unknown. https://cursor.com/docs/agent/hooks

## 7. Hooks and events

- **[docs]** General Cursor agent event inventory: `sessionStart`, `sessionEnd`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `subagentStart`, `subagentStop`, `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preCompact`, `stop`, `afterAgentResponse`, `afterAgentThought`. Tab-only `beforeTabFileRead`,`afterTabFileEdit`; IDE-only `workspaceOpen`. **CLI parity for each event is unknown**; this is the shared Cursor reference, not a local CLI validation. https://cursor.com/docs/agent/hooks
- **[docs]** Config `version:1,hooks:{event:[{command,type?,timeout?,matcher?,loop_limit?,failClosed?}]}`; command hooks exchange JSON stdio, prompt hooks use LLM checks (`{ok,reason?}`) and must not be used in a no-model-call probe. Enterprise → team → project `.cursor/hooks.json` → user `~/.cursor/hooks.json` → plugin sources all contribute. Deny > ask > allow; text context concatenates; some other fields last-wins. https://cursor.com/docs/agent/hooks
- **[docs]** Common payload: `conversation_id`, `generation_id`, `model`, optional `model_id`,`model_params`, `hook_event_name`, `cursor_version`, `workspace_roots`, `user_email`, `transcript_path`. Tool events add `tool_name`,`tool_input`,`tool_use_id`,`cwd`; post adds `tool_output` JSON string and `duration`; failure adds `error_message`,`failure_type:timeout|error|permission_denied`,`is_interrupt`. https://cursor.com/docs/agent/hooks
- **[docs]** Pre-tool output supports `permission`, `updated_input`, `user_message`,`agent_message`; `ask` accepted but not enforced in generic preToolUse. Post output supports `additional_context`, MCP-only `updated_mcp_tool_output`. Shell-specific permission can ask/allow/deny. `subagentStart` can deny; `subagentStop` includes status, task, summary, duration, counts, modified files, agent transcript and can return `followup_message` on completed status. https://cursor.com/docs/agent/hooks
- **[docs]** Stop input includes `status:completed|aborted|error`,`loop_count`; output `followup_message` re-enters agent. Per-script stop/subagentStop loop limit defaults five (`null` unlimited). Exit 2 blocks; other errors/timeouts fail open unless `failClosed:true`. Permission hooks with malformed response block even without failClosed. Output-size cap, exact default timeout and per-process CLI hook configuration flag: unknown. https://cursor.com/docs/agent/hooks

## 8. Plugins and extensions

- **[docs]** Repeatable `--plugin-dir PATH` loads local plugins for one invocation. Plugins can contribute command hooks; `CURSOR_PLUGIN_ROOT` identifies installation root in hooks. `workspaceOpen` can return plugin paths but is IDE lifecycle, not proof of CLI support. https://cursor.com/docs/cli/reference/parameters https://cursor.com/docs/agent/hooks
- Full executable plugin registration surface (tools/UI/providers), unloading, isolation, CLI-supported manifest components: unknown.

## 9. Tools, permissions, and approvals

- **[docs]** Print mode has all tools, including writes and shell. `--force`/`--yolo` allows commands unless explicitly denied. `--approve-mcps` auto-approves servers; `--trust` skips workspace trust headlessly. https://cursor.com/docs/cli/reference/parameters
- **[docs]** Sandbox CLI `agent sandbox enable|disable|reset|run`; run flags `--allow-paths`, `--readonly-paths`, `--blocked-patterns`, `--sandbox` (true), `--network` (false), `--sb-debug`. Startup `--sandbox enabled|disabled` selects process mode. https://cursor.com/docs/cli/reference/parameters
- Programmatic approval/question response via print stdin, precise allow/deny config syntax and ACP permission specifics: unknown; do not confuse force with OS isolation.

## 10. Subagents and delegation

- **[docs]** Shared hooks describe Task subagents `generalPurpose`, `explore`, `shell` and `is_parallel_worker`; `subagentStart` can deterministically deny creation. CLI availability/restriction flags and actual event coverage: unknown. https://cursor.com/docs/agent/hooks

## 11. MCP

- **[docs]** `agent mcp list`, `list-tools IDENTIFIER`, `login IDENTIFIER`, `enable IDENTIFIER`, `disable IDENTIFIER`; list-tools includes argument names; enable/disable changes local approved list. Config is workspace/user JSON mentioned above. https://cursor.com/docs/cli/reference/parameters
- Transport matrix, resources/prompts/elicitation, externally initiated session messages and process-only MCP JSON injection: unknown.

## 12. Control from outside

- **[docs]** Hidden advanced `agent acp` starts ACP server; do not claim no native protocol merely because absent from default help. Exact ACP methods/transport/capabilities unknown in checked reference. https://cursor.com/docs/cli/reference/parameters
- **[docs]** `agent worker start` is private cloud worker, not a generic attach-to-local-TUI server. Flags: `--auth-token-file`, repeated `--worker-dir`, `--management-addr` (`/healthz`,`/readyz`,`/metrics`), repeated `--label` or `--labels-file`/`CURSOR_WORKER_LABELS_FILE`, `--idle-release-timeout` (0), `--pool`/legacy `--single-use`, `--pool-name`, `--name`, `--data-dir`, `--debug`, `--computer-use`, `--display`, `--share-desktop view|view_and_control`; start `--verbose`, debug `--json`. https://cursor.com/docs/cli/reference/parameters
- Running interactive-turn injection/steering/interrupt API and TUI attachment to ACP process: unknown.

## 13. Status from outside

- **[docs]** Stream start/tool events and terminal result expose run progress; nonzero exit/stderr is required to detect failure, even in JSON mode. Worker health/ready/metrics are separate worker-level signals, not chat busy/idle. https://cursor.com/docs/cli/reference/output-format https://cursor.com/docs/cli/reference/parameters
- Shared hooks expose permission and stop statuses, but CLI coverage unknown; no independently verified live state endpoint.

## 14. Usage, cost, and limits

- **[docs]** Print result exposes duration, not token/cost fields in published schema. https://cursor.com/docs/cli/reference/output-format
- Token/context/rate/cost readout API: unknown; do not infer usage from duration_api_ms (currently equals duration_ms).

## 15. Authentication

- **[docs]** `agent login|logout`, `agent status|whoami --format text|json`, `agent about --format text|json`; flags `--api-key` and `CURSOR_API_KEY`. Status is the documented non-model auth inspection route. Credential storage and expiry guarantees unknown. https://cursor.com/docs/cli/reference/parameters

## 16. Running many instances

- **[docs]** Native `--worktree [name]` creates `~/.cursor/worktrees/<reponame>/<name>`, optional base branch; `.cursor/worktrees.json` setup scripts run unless `--skip-worktree-setup`. https://cursor.com/docs/cli/reference/parameters
- Shared credential/session file locks, RAM, rate quotas and same-directory conflicts: unknown. Distinct worker `--data-dir` and management address avoid some path/port collision, but do not establish account isolation.

## 17. Anything else a program driving this harness should know

- **[docs]** Best documented script interface is print NDJSON; success-only final JSON is not sufficient error reporting. Partial events include duplicates. Native ACP exists but is explicitly advanced/hidden. https://cursor.com/docs/cli/reference/output-format https://cursor.com/docs/cli/reference/parameters
