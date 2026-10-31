# Aider

## 1. Overview

- **[verified]** Research date: 2026-10-30. `aider` was not on PATH; nothing installed and no model calls made. Consequently there is no locally checked runtime version.
- **[docs]** Latest GitHub release returned by the API: `v0.86.0`, published 2025-08-09. This is a release observation, not proof that current main/docs equal that release. https://api.github.com/repos/Aider-AI/aider/releases/latest
- **[docs]** Aider-AI's Python terminal pair programmer, Apache-2.0; install with `python -m pip install aider-chat` (installation was not performed). Release cadence and terminal-only active usage: unknown. https://github.com/Aider-AI/aider

## 2. Interactive terminal interface

- **[docs]** Inline chat with configurable fancy input, multiline and VI editing: `--[no-]fancy-input`, `--multiline`, `--vim`; `--[no-]pretty`, `--[no-]stream`, color flags and `--code-theme` control rendering. `--gui`/`--browser` launches browser UI instead. https://aider.chat/docs/config/options.html
- **[docs]** `--notifications` enables completion bell; `--notifications-command COMMAND` selects external notification delivery instead. https://aider.chat/docs/config/options.html
- Alternate screen, mouse, exact title/OSC 9/9;4/7501 sequences and resize behavior: unknown; not captured locally.

## 3. Launch and configuration

- **[docs]** `--config/-c FILE`, `--env-file FILE`, repeated `--set-env NAME=value`, repeated `--api-key PROVIDER=KEY`; options generally have `AIDER_` equivalents (hyphens become underscores), explicitly documented per option. Default YAML search covers home, Git root, cwd `.aider.conf.yml`; `.env` defaults to Git root. https://aider.chat/docs/config/options.html
- **[docs]** Process controls include `--file FILE`, repeated `--read FILE`, `--load FILE` (execute slash commands at startup), `--encoding`, `--line-endings`, `--chat-language`, `--commit-language`, `--editor`, `--timeout`, `--disable-playwright`. https://aider.chat/docs/config/options.html
- **[docs]** File/Git controls: `--[no-]git`, `--[no-]gitignore`, `--[no-]add-gitignore-files`, `--aiderignore FILE`, `--subtree-only`, `--[no-]auto-commits`, `--[no-]dirty-commits`, attribution flags, `--[no-]git-commit-verify`, `--dry-run`, `--watch-files`, `--skip-sanity-check-repo`. https://aider.chat/docs/config/options.html
- **[docs]** Output controls include individual user/tool/error/warning/assistant/completion-menu foreground and background colors, dark/light mode, show diffs, verbosity; update/telemetry controls `--[no-]check-update`, `--[no-]show-release-notes`, `--[no-]analytics`, `--analytics-log FILE`. Do not use `--analytics-disable` as a temporary switch: it permanently disables analytics. https://aider.chat/docs/config/options.html
- Complete config-layer precedence and whether an explicit config excludes all defaults: unknown in sources checked; do not assume setting a history path isolates configuration discovery.

## 4. Models and reasoning

- **[docs]** `--model`, `--weak-model` (commit messages and summarization), `--editor-model`, `--architect`, `--edit-format`, `--editor-edit-format`, `--[no-]auto-accept-architect`, `--reasoning-effort`, `--thinking-tokens`; zero thinking tokens disables reasoning where supported. https://aider.chat/docs/config/options.html
- **[docs]** `--list-models/--models PARTIAL` lists known matches; repeat `--alias ALIAS:MODEL`. Unknown-model metadata can be supplied with `--model-settings-file` (default `.aider.model.settings.yml`) and `--model-metadata-file` (default `.aider.model.metadata.json`). Warnings/settings checks use `--[no-]show-model-warnings` and `--[no-]check-model-accepts-settings`. Exact invalid-provider/model failure timing unknown. https://aider.chat/docs/config/options.html
- **[docs]** OpenAI-compatible endpoint and authentication overrides: `--openai-api-base`, `--openai-api-key`, `--anthropic-api-key`; other providers through `--api-key`/environment. https://aider.chat/docs/config/options.html

## 5. Conversations

- **[docs]** No session-ID interface is advertised in the complete options reference. Persistence is file-based: `--input-history-file` defaults `.aider.input.history`, `--chat-history-file` defaults `.aider.chat.history.md`; `--restore-chat-history` reloads previous messages and defaults false. https://aider.chat/docs/config/options.html
- **[docs]** `--max-chat-history-tokens` sets a soft summarization threshold. https://aider.chat/docs/config/options.html
- Create chosen ID, fork/list/export/delete API, same-history concurrent writer guarantees: unknown. A driver can choose distinct transcript filenames, but that is not a native session ID.

## 6. Transcript and structured output

- **[docs]** Chat transcript is Markdown; `--llm-history-file FILE` logs actual model conversation, and `--show-diffs` shows diffs at commit. `--analytics-log FILE` is a separate event log, not a promised complete transcript. https://aider.chat/docs/config/options.html
- **[docs]** `--message/-m/--msg TEXT` or `--message-file/-f FILE` runs one instruction, applies edits, then exits; `--no-pretty --no-stream --no-fancy-input` simplify text parsing. There is no JSON/NDJSON result flag in the options reference. https://aider.chat/docs/config/options.html
- Structured plans/todos, stable event schema, transcript flush/durability behavior while live: unknown.

## 7. Hooks and events

- **[docs]** Custom lint and test execution: repeated `--lint-cmd`, `--test-cmd`, `--[no-]auto-lint` (default true), `--[no-]auto-test` (default false), `--lint`, `--test`; notification command above. These are specific callbacks, not a general hook event bus. https://aider.chat/docs/config/options.html
- General lifecycle hook names/payloads, sync/async policy, interception, hook permissions and output limits: unknown; no such interface in checked options/scripting docs.

## 8. Plugins and extensions

- **[docs]** Python embedding: `from aider.coders import Coder`, `from aider.models import Model`; `Coder.create(main_model=Model(...), fnames=[...])`, then repeated `coder.run(TEXT)`; slash commands such as `coder.run('/tokens')` also work. `InputOutput(yes=True)` provides approval defaults. The scripting API is explicitly unsupported and may change without compatibility. https://aider.chat/docs/scripting.html
- Stable registration API for tools/UI/providers/agents or process-scoped plugin directory: unknown.

## 9. Tools, permissions, and approvals

- **[docs]** `--yes-always` answers every confirmation yes; `--dry-run` disables file modifications, not a documented security sandbox. `--read FILE` adds read-only context; shell suggestions controlled by `--[no-]suggest-shell-commands`. https://aider.chat/docs/config/options.html
- **[docs]** Debug edit application without model inference: `--apply FILE`, `--apply-clipboard-edits`; `--exit` performs startup and exits. Startup is not documented as side-effect-free. https://aider.chat/docs/config/options.html
- Structured external approval/question answering, allow/deny command policies, OS sandbox: unknown.

## 10. Subagents and delegation

- **[docs]** Architect/editor is a two-model editing workflow; weak model handles auxiliary work. These are not evidence of background autonomous subagents. https://aider.chat/docs/config/options.html
- Native subagents, teams, background job control, disable flag: unknown.

## 11. MCP

- Native client transports/resources/prompts/elicitation/push and per-process MCP config: unknown; not present in checked complete option reference. Do not infer support from third-party bridges.

## 12. Control from outside

- **[docs]** Supported CLI scripting is one instruction per process via `--message`; Python `Coder.run()` permits multiple turns in one embedding process, with the compatibility warning above. `--watch-files` provides an alternate externally editable input surface for AI comments. https://aider.chat/docs/scripting.html https://aider.chat/docs/config/options.html
- Public server/RPC/ACP, attach, active-turn steering, structured interrupt: unknown.

## 13. Status from outside

- Completion/process exit and transcript/terminal observation are available; machine-readable busy/idle/approval/input/error status protocol: unknown. Notification callback indicates ready, not a general state snapshot. **[docs]** https://aider.chat/docs/config/options.html

## 14. Usage, cost, and limits

- **[docs]** `/tokens` can be executed through Python `coder.run`; `--map-tokens`, `--map-refresh`, `--map-multiplier-no-files` govern repo context; `--cache-prompts`, `--cache-keepalive-pings` affect provider cache behavior. Keepalive defaults zero; positive values request pings at five-minute intervals. https://aider.chat/docs/scripting.html https://aider.chat/docs/config/options.html
- Stable structured cost/rate-limit accounting API: unknown.

## 15. Authentication

- **[docs]** Provider credentials via API-key flags, provider environment and configurable `.env`; `--set-env` also passes API settings. https://aider.chat/docs/config/options.html
- A credential-presence check is not proof of validity. Uniform no-model-call logged-in/expired detection and credential-store layout across providers: unknown.

## 16. Running many instances

- **[docs]** Each process can select separate history files and turn off automatic/dirty commits; default history files and Git actions are project-local, so sharing those paths does not isolate instances. https://aider.chat/docs/config/options.html
- Locking, memory footprint, per-instance ports and model-provider limits: unknown. Use separate worktrees/history paths rather than assume repository writes are serialized.

## 17. Anything else a program driving this harness should know

- **[docs]** Auto commits and dirty commits default true; `--gitignore` defaults true, so launch can modify repository metadata. Deliberately choose `--no-auto-commits --no-dirty-commits --no-gitignore` when the parent program owns Git state. https://aider.chat/docs/config/options.html
- **[docs]** Older scripting docs spell approval `--yes`; current option reference spells `--yes-always`. Version-check the installed executable rather than blindly copy old examples. https://aider.chat/docs/scripting.html https://aider.chat/docs/config/options.html
