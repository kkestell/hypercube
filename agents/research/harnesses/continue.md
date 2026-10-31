# Continue CLI (`cn`)

## 1. Overview

- **[verified]** Research 2026-10-30; `cn` not on PATH. No runtime version/install/model call.
- **[docs]** Continue's Apache-2.0 customizable terminal coding agent; npm `@continuedev/cli` (Node20+) or official platform install scripts. Main package version is `0.0.0-dev`; repo latest `v2.0.0-vscode` (2026-06-19) is **not CLI version**. Release cadence/current published CLI version unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/package.json https://api.github.com/repos/continuedev/continue/releases/latest https://api.github.com/repos/continuedev/continue
- **[docs]** Selection proxy, current repo stars: Cline71,082; Continue36,379; Crush28,890 selected; Qwen28,840; Kilo27,807; archived Python Kimi11,422 rejected. GitHub totals include IDE audience, not CLI usage. Factory Droid no comparable evidence established; exact most-used terminal ranking unknown. https://api.github.com/repos/cline/cline https://api.github.com/repos/continuedev/continue https://api.github.com/repos/charmbracelet/crush https://api.github.com/repos/QwenLM/qwen-code https://api.github.com/repos/Kilo-Org/kilocode https://api.github.com/repos/MoonshotAI/kimi-cli

## 2. Interactive terminal interface

- **[docs]** TUI default; `-p/--print` skips TUI/TTY input with supplied prompt; `FORCE_NO_TTY` forces no-TTY behavior. Source SIGINT interactive handler exits only second Ctrl+C within1000ms; non-TUI SIGINT exits130; SIGTERM graceful exit0 handler. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts
- Fullscreen/alternate screen, mouse, title/OSC/bell and resize details unknown; no PTY test.

## 3. Launch and configuration

- **[docs]** Root `-p/--print`, `--format json` (only print), `--silent` strips think tags/whitespace (only print), `--resume`, `--fork SESSION_ID`, `--beta-subagent-tool`, positional prompt. Piped stdin + prompt in headless mode produces `<stdin>...input...</stdin>\n\nPROMPT`; otherwise piped input can start TUI first prompt. Print requires prompt unless agent/resume. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts
- **[docs]** Shared flags `--config PATH_OR_HUB_SLUG`, `--org SLUG` (headless only), `--readonly`, `--auto`, `--verbose`, `--beta-status-tool`, repeatable `--rule FILE|SLUG|STRING`, `--mcp OWNER/PACKAGE`, `--model OWNER/PACKAGE`, `--prompt FILE|SLUG|STRING`, `--allow TOOL`, `--ask TOOL`, `--exclude TOOL`, `--agent OWNER/PACKAGE`. `--model` adds model package, not necessarily arbitrary provider raw ID. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/shared-options.ts
- **[docs]** `CONTINUE_GLOBAL_DIR` relocates home state, `CONTINUE_CLI_DISABLE_COMMIT_SIGNATURE` disables attribution, `FORCE_NO_TTY`. Hook config also reads `.claude` settings, so Continue root alone does not isolate foreign hooks. Full model YAML/config policy precedence not established. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/session.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/hookConfig.ts

## 4. Models and reasoning

- **[docs]** `--model SLUG` repeated hub packages; `--config` agent config, `--agent` hub agent. No standalone raw `--provider`/`--reasoning-effort` flag in current shared option source. Provider YAML/effort keys, model enumeration and unknown-model behavior unknown in checked sources. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/shared-options.ts

## 5. Conversations

- **[docs]** Automatic saved history per terminal; `--resume` latest terminal session, `cn ls` picker, `cn ls --json` always10 sessions. `--fork SESSION_ID` root option. Session JSON `<CONTINUE_GLOBAL_DIR or ~/.continue>/sessions/<sessionId>.json`; history manager list/load/save; default UUID creation. Source additionally checks HOME sessions for some migration/discovery logic, so root isolation is not universal proof. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/session.ts
- Public caller-chosen create ID, delete/export CLI and concurrent-writer guarantee unknown. Internal createSession(history,sessionId?) exists but is not a documented stable external API. **[docs]** https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/session.ts

## 6. Transcript and structured output

- **[docs]** Headless `cn -p TEXT --format json`; session files store history. HTTP `/state` synchronizes service history then returns complete state snapshot; source uses service as authority, no duplicate writes into history during content callbacks. Exact JSON headless result schema and live partial-state completeness unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.helpers.ts
- **[docs]** GET `/diff` returns Git diff against main. Plan/todo stable structured output unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts

## 7. Hooks and events

- **[docs]** Declared event names: `PreToolUse`,`PostToolUse`,`PostToolUseFailure`,`PermissionRequest`,`UserPromptSubmit`,`SessionStart`,`SessionEnd`,`Stop`,`Notification`,`SubagentStart`,`SubagentStop`,`PreCompact`,`ConfigChange`,`TeammateIdle`,`TaskCompleted`,`WorktreeCreate`,`WorktreeRemove`. This is the **type inventory**, not a local proof every path emits every event. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Sources low→high: `~/.claude/settings.json`, `<Continue home>/settings.json`, project `.claude/settings.json`, `.continue/settings.json`, `.claude/settings.local.json`, `.continue/settings.local.json`; merges hooks, `disableAllHooks` can clear. Shape `{hooks:{EVENT:[{matcher?:REGEX,hooks:[HANDLER]}]},disableAllHooks?}`. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/hookConfig.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Handler types command `{command,async?}`, http `{url,headers?,allowedEnvVars?}`, prompt/agent `{prompt,model?}`; common `timeout,statusMessage,once` (skills-only once). Timeout defaults documented in types: command600sec, http/prompt30sec, agent60sec. Prompt/agent hooks imply model calls; not probed. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Common input `{session_id,transcript_path,cwd,permission_mode?}` plus hook_event_name. Pre `{tool_name,tool_input,tool_use_id}`; post adds tool_response; failure `{error,is_interrupt?}`; permission tool/input; prompt prompt; start `{source:startup|resume|clear|compact,agent_type?,model?}`; end `{reason:clear|logout|prompt_input_exit|other|bypass_permissions_disabled}`; stop `{stop_hook_active,last_assistant_message?}`; notification `{message,title?,notification_type}`. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Subagent start `{agent_id,agent_type}`; stop adds `stop_hook_active,agent_transcript_path,last_assistant_message?`; compact `{trigger:manual|auto,custom_instructions:string|null}`; config `{source,file_path?}`; teammate `{teammate_name,team_name}`; task `{task_id,task_subject,task_description?,teammate_name?,team_name?}`; worktree create name/remove worktree_path. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Output `{continue?,suppressOutput?,stopReason?,decision:approve|block?,systemMessage?,reason?,hookSpecificOutput?}`. Pre-specific `{hookEventName:'PreToolUse',permissionDecision:allow|deny|ask,permissionDecisionReason?,updatedInput?,additionalContext?}`; permission output decision `{behavior:'allow',updatedInput?}` or `{behavior:'deny',message?,interrupt?}`. Post may replace `updatedMCPToolOutput`, plus context; prompt/start/failure/notification/subagent-start may add context. Exit2 or decision block = blocked per execution-result type. Stop loop limits, output cap and actual HTTP/prompt/agent executor support not independently established. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
- **[docs]** Matcher fields: tool events/permission `tool_name`, start source, end reason, notification notification_type, subagent agent_type, compact trigger, config source; prompt/stop/teammate/task/worktree have no matcher. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts

## 8. Plugins and extensions

- **[docs]** Hub agents/models/rules/prompts/MCP via per-process flags above. Hook compatibility loads Claude settings as well as Continue settings. Arbitrary runtime plugin registration API for UI/tools/providers/agents and plugin-dir flag unknown; do not substitute editor extension APIs without checking CLI. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/shared-options.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/hookConfig.ts

## 9. Tools, permissions, and approvals

- **[docs]** `--readonly` plan read-only tools, `--auto` all tools allowed; repeated `--allow`,`--ask`,`--exclude` override default policies. Fine-grained file policy grammar and precedence among explicit flags unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/shared-options.ts
- **[docs]** HTTP permission request `{toolName,toolArgs,requestId,timestamp,toolCallPreview?}` stored in pendingPermission. POST `/permission` `{requestId,approved}`; rejects400 if no pending or mismatched ID; resolves manager and clears state, returns `{success:true,approved}`. Hooks can decide permissions as above. OS sandbox and external user-question answering API unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.helpers.ts

## 10. Subagents and delegation

- **[docs]** `--beta-subagent-tool` explicitly enables beta Subagent tool. Omit rather than assume parallel-team support from hook type names alone. Tool exclusion and hook intercept can restrict; actual teams/background job semantics unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts

## 11. MCP

- **[docs]** Repeated `--mcp OWNER/PACKAGE` adds hub MCP packages for session; arbitrary server YAML config via `--config`. Native client transport/resource/prompt/elicitation/push matrix and inline JSON alternative unknown in checked sources. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/shared-options.ts

## 12. Control from outside

- **[docs]** Hidden `cn serve [prompt] --port PORT` default8000, `--timeout SECONDS` inactivity default300, `--id STORAGE_ID` uploads snapshots to Continue storage, `--beta-upload-artifact-tool`; inherits shared options. Server uses Express `app.listen(port)` without explicit loopback host in checked code; no auth middleware shown around endpoints, so treat as potentially network-accessible and protect externally. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts
- **[docs]** GET `/state` resets inactivity timestamp and returns snapshot; POST `/message` `{message}` required (400 otherwise), queues message, returns `{queued:true,position}`, starts processing if idle; POST `/pause` aborts controller and clears isProcessing, returns `{success:true,message:'Agent run paused'}` or `{success:false,message:'No active processing to pause'}`; GET `/diff`; POST `/exit`; POST `/permission` as above. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts
- **[docs]** Interruption wrapper checks queue predicate every100ms then aborts, supporting incoming-message steering rather than just launching new processes. Exact queue interruption policy unknown outside wrapper; do not promise token-immediate cancellation. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.helpers.ts
- README mentions login/logout/remote but current index imports do not register them in viewed source; exact version-specific availability unknown. ACP/SDK or attach-TUI-to-serve not established.

## 13. Status from outside

- **[docs]** Server internal state `{session,config,model,isProcessing,lastActivity,currentAbortController,serverRunning,pendingPermission,systemMessage?}`; `/state` snapshot derived from session,isProcessing,queue length,pendingPermission. Pending permission shape above permits authoritative approval wait vs model work. Exact snapshot JSON key set unknown, not equated blindly to internal state. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.helpers.ts
- **[docs]** In serve mode `--id`, unhandled exceptions/rejections POST `agents/<id>/status` `{status:'FAILED',errorMessage}` to Continue API, log and mark eventual nonzero error; handlers do not immediately exit. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts

## 14. Usage, cost, and limits

- Stored session/history and snapshots may carry usage; exact token/cost schema/rate-limit API unknown. Inactivity timeout is not a per-turn token limit. **[docs]** https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts

## 15. Authentication

- **[docs]** README advertises `cn login` and `cn logout`; source command surface differs as noted. Provider key locations, no-call auth/expiry probe and headless hub credentials unknown. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/README.md

## 16. Running many instances

- **[docs]** Choose distinct server `--port`, Continue roots/session files and worktrees; module-global messageQueue/services make single process shared state relevant. `CONTINUE_GLOBAL_DIR` affects primary session paths but `.claude` hooks still read. Polling `/state` refreshes inactivity timer, so monitoring itself keeps server alive. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/commands/serve.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/session.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/hookConfig.ts
- File locks/memory/profile-shared quotas and same-directory mutation guarantees unknown.

## 17. Anything else a program driving this harness should know

- **[docs]** Current root also registers `checks [action] [pr-url]` and `review --base REF --format FORMAT --fix --patch --fail-fast --review-agents ... --verbose`; internal `--internal-review-worker` bypasses ordinary parser and invokes review worker. Do not use `--fix` during read-only discovery. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts
- **[docs]** README/source disagreements and dev package version mean command help on installed build is essential. Defined hook events are not proof of actual team/worktree runtime capability. https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/index.ts https://raw.githubusercontent.com/continuedev/continue/main/extensions/cli/src/hooks/types.ts
