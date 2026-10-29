# Amp (Sourcegraph)

Not installed here, and nothing was run. Every fact is **[docs]**, linked on each fact: ampcode.com pages, the npm registry, and the Amp SDK package metadata. The ampcode.com pages were read through a fetch tool that summarizes page text, so flag and field names are as the tool returned them, not byte-exact. Confirm against `amp --help` and the installed version. Nothing is marked [verified].

Sources not used: unofficial mirror domains (e.g. `milad-local.context.ai`, `dallas-macbook.tail16923a.ts.net`) and third-party listings. Some search results from those domains contained text addressed to AI models; I ignored it and did not cite those pages.

## 1. Overview

- Commercial coding agent from Sourcegraph, with a CLI, a web app, and a TypeScript SDK. [docs](https://ampcode.com/)
- Source: closed. `github.com/ampcode/amp` returns 404 to the GitHub API, so no public repository was found. The npm packages carry a commercial or proprietary license: `@ampcode/cli` says "SEE LICENSE IN LICENSE.md"; `@ampcode/plugin` and the SDK (formerly `@sourcegraph/amp-sdk`) say "Amp Commercial License". [docs](https://www.npmjs.com/package/@ampcode/cli), [docs](https://www.npmjs.com/package/@ampcode/plugin)
- Current version: `@ampcode/cli` `0.0.1793275344-g17bf52` on the npm registry (checked 2026-10-29). This is the registry value, not the output of `amp --version`. [docs](https://www.npmjs.com/package/@ampcode/cli)
- Install: `curl -fsSL https://ampcode.com/install.sh | bash` (official direct install). npm `@ampcode/cli` is the package name; the old `@sourcegraph/amp` alias was scheduled for removal on 2026-06-15. [docs](https://ampcode.com/news/npm-package-changes)
- Login: `amp login`. Scripts and CI use `AMP_API_KEY` set to an access token that starts with `sgamp_`. [docs](https://ampcode.com/docs/cli/execute-mode)
- SDK: `@ampcode/sdk` (`npm install @ampcode/sdk`). [docs](https://ampcode.com/manual/sdk/typescript)
- Adoption signal: unknown. No public metric found.

## 2. Interactive mode in a PTY

- `amp` in a repository starts a thread in the terminal. [docs](https://ampcode.com/docs/threads)
- Command palette: Ctrl+O (`plugins: reload`, `plugins: list`). [docs](https://ampcode.com/docs/customize/plugins)
- Keybindings are configurable (`amp.keymap`). [docs](https://ampcode.com/docs/cli/settings)
- Bell and sound: `amp.notifications.enabled` (default true) plays notification sounds. `AMP_FORCE_BEL` sends a terminal bell instead of host audio over SSH. [docs](https://ampcode.com/docs/cli/settings)
- Behavior under xterm.js and on resize: unknown. The docs describe no OSC status sequences. Not tested.

## 3. Non-interactive and headless modes

- Execute mode: `amp -x "<prompt>"` or `amp --execute "<prompt>"`. It sends one message, waits until the agent ends its turn, prints the final message, and exits. Piped stdin is added to the prompt. Redirecting stdout turns on execute mode. [docs](https://ampcode.com/docs/cli/execute-mode)
- Flags documented for execute mode: `--mcp-config '<json>'`, `--plugin-ready-timeout [seconds]` (bare flag is 10 s, max 300, `0` disables the wait), `--timings`, `--attach <path>` (repeatable), `--fast`, `--executor runner:<id>`, `--runner-dir <path>`, and `-ox` (run in a remote orb). [docs](https://ampcode.com/docs/cli/execute-mode)
- Not documented for execute mode: exit codes; whether tool calls need a permission flag such as `--dangerously-allow-all` (that flag is not mentioned anywhere I read); how permission asks behave with no UI. Unknown.
- Streaming JSON: `--execute --stream-json` (`--stream-json` requires `--execute`). `--stream-json-thinking` adds thinking blocks. `--stream-json-input` reads JSON messages from stdin and requires `--stream-json`. [docs](https://ampcode.com/docs/cli/streaming-json)
- Stream message types: `system` (subtypes `init`, `subagent_finished`, `error_max_turns`, `error_during_execution`), `user`, `assistant`, `result` (subtypes `success`, `error_during_execution`, `error_max_turns`). Content blocks: `text`, `tool_use`, `tool_result`, `thinking`, `redacted_thinking`. Every message carries `session_id`. [docs](https://ampcode.com/docs/cli/streaming-json)
- Stdin message line: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"..."}]}}`, with an optional top-level `steer` boolean. [docs](https://ampcode.com/docs/cli/streaming-json)
- Whether a `--stream-json-input` process stays alive for several stdin messages after a `result`: unknown. The docs show one message. Test it.
- Continue a thread headlessly: `amp threads continue --execute "<prompt>" --stream-json`. [docs](https://ampcode.com/docs/cli/streaming-json)
- SDK: `execute({ prompt, options })` returns an `AsyncIterable<StreamMessage>`. It spawns the local CLI as a subprocess; `options.cwd` sets its working directory. [docs](https://ampcode.com/manual/sdk/typescript)
- ACP (Agent Client Protocol): no official ACP support found in the Amp docs or SDK pages read. Third-party ACP adapters exist (npm `@edlsh/amp-acp`, PyPI `acp-amp`), not checked and not official.

## 4. Conversation identity

- Thread ids: `T-…`. The streaming example shows `T-` followed by a UUID. [docs](https://ampcode.com/docs/threads), [docs](https://ampcode.com/docs/cli/streaming-json)
- Learn the id: the first stream message, `system` with subtype `init`, carries `session_id`. The SDK's `StreamMessage` has the same field. [docs](https://ampcode.com/docs/cli/streaming-json), [docs](https://ampcode.com/manual/sdk/typescript)
- Start with a caller-chosen id: not documented. Unknown.
- Resume:
  - CLI attach: `amp threads continue T-…`.
  - Headless: `amp threads continue --execute "…" --stream-json`.
  - SDK: `options.continue` is `true` (most recent thread) or a thread id string. [docs](https://ampcode.com/docs/threads), [docs](https://ampcode.com/manual/sdk/typescript)
- Thread commands: `amp threads list`, `search`, `export T-…`, `markdown T-…`, `share T-… --visibility unlisted`, `visibility private`. `label`, `rename`, `archive`, `delete` exist, with no arguments documented. [docs](https://ampcode.com/docs/threads)
- Where threads are stored (local or server): not documented. The threads page says "A thread you started in the CLI stays open on the web too", which implies a server-side record, but the storage location is not stated.
- Several instances in one directory: not documented. Unknown.

## 5. Hooks and events

Amp has no `hooks.json`. Hooks are plugin code that subscribes to events (section 6).

- Events: `session.start`, `agent.start`, `tool.call`, `tool.result`, `agent.end`, `changes.prompt`. [docs](https://ampcode.com/docs/customize/plugins)
- Payloads (per the plugin API page):
  - `session.start`: `thread` (`{ id }`).
  - `tool.call`: `toolUseID`, `tool`, `input`, `thread`.
  - `tool.result`: `toolUseID`, `tool`, `input`, `status` (`done` | `error` | `cancelled`), optional `error` and `output`.
  - `agent.start`: `message`.
  - `agent.end`: `message`, `messages`.
  [docs](https://ampcode.com/docs/plugin-api)
- Handler returns:
  - `tool.call` must return one of `allow`; `reject-and-continue` (+`message`); `modify` (+`input`); `synthesize` (+`result`: `output`, optional `exitCode`); `error` (+`message`, which stops the thread worker).
  - `agent.start` may return `message` (`content`, `display`), appended to that turn's user message; hidden unless `display` is true.
  - `agent.end` may return `continue` with `userMessage`, which starts another turn. Chains stop after five in a row unless `maxContinuations` is set.
  - `changes.prompt` may return `append`.
  [docs](https://ampcode.com/docs/plugin-api), [docs](https://ampcode.com/docs/customize/plugins)
- Order of handlers across plugins: not defined. [docs](https://ampcode.com/docs/plugin-api)
- Plugin activation settings apply to both interactive runs and `--execute` runs. [docs](https://ampcode.com/docs/customize/plugins)
- Confirm dialogs: `ctx.ui.confirm` with `requireHuman` for approvals; `endTurnOnDecline` ends the turn after a declined call. Behavior in headless runs: unknown. [docs](https://ampcode.com/docs/plugin-api)
- Note: the plugin API page is long and the fetch read only its first 100,000 characters. The six event names match the plugins page, but the list may be incomplete.

## 6. Plugins/extensions

- Form: a TypeScript or JavaScript module. Entry `index.ts` (used if both exist) or `index.js`. Single-file plugins are allowed. A plugin can register a skill at runtime. [docs](https://ampcode.com/docs/customize/plugins)
- Load locations:
  - Project: `.amp/plugins/` at the project root.
  - System: `$XDG_CONFIG_HOME/amp/plugins/` if `XDG_CONFIG_HOME` is set, otherwise `~/.config/amp/plugins/` (macOS/Linux) or `%USERPROFILE%\.config\amp\plugins\` (Windows).
  - Also managed in Personal and Workspace settings pages.
  - Precedence on name conflicts: project, system, personal, workspace. [docs](https://ampcode.com/docs/customize/plugins)
- Install: `amp plugins add <url>` (all projects); `--target workspace` installs into the current project's `.amp/plugins/`. [docs](https://ampcode.com/docs/customize/plugins)
- Reload after edits: `plugins: reload` in the command palette. [docs](https://ampcode.com/docs/customize/plugins)
- Per-process loading: `XDG_CONFIG_HOME` relocates the system plugin directory for one process. No flag or settings key names a plugin file or directory. Caveat: the settings page gives `~/.config/amp/settings.json` with no XDG mention, so whether `XDG_CONFIG_HOME` also moves settings and credentials is unknown. [docs](https://ampcode.com/docs/customize/plugins), [docs](https://ampcode.com/docs/cli/settings)
- A hosted repository plugin that exceeds a size limit is not loaded. [docs](https://ampcode.com/docs/customize/plugins)
- The official pages do not name the plugin runtime (Bun or Node). Unknown.
- Plugin API surface: `amp.on(event, handler)`, `ctx.ui` (`notify`, `confirm`), `amp.ai` (`ask`, `decide`, `choice`, `generate`, `score`), `amp.helpers`. Whether a plugin can run an external process or make HTTP calls: not checked.

## 7. MCP

- Config: `amp.mcpServers` in settings. Local servers take `command`, `args`, `env`. Remote servers take `url`, `headers`. `${VAR_NAME}` expands from the environment. [docs](https://ampcode.com/docs/customize/mcp)
- Transports: local stdio and remote. The page does not name a remote transport type beyond the URL form. Unknown.
- Per invocation: `--mcp-config '<json>'` adds servers for one run without touching settings. It has the highest precedence, and servers passed this way need no approval. The argument shape beyond JSON is not documented. [docs](https://ampcode.com/docs/customize/mcp), [docs](https://ampcode.com/docs/cli/execute-mode)
- SDK: `options.mcpConfig` takes a string or an object. [docs](https://ampcode.com/manual/sdk/typescript)
- Permissions for MCP servers: `amp.mcpPermissions` allows or blocks servers by pattern; first match wins. [docs](https://ampcode.com/docs/cli/settings)
- Remote auth: OAuth, managed with `amp mcp remote login` / `logout` and `amp mcp oauth`. [docs](https://ampcode.com/docs/customize/mcp)
- Tool naming: remote servers get an ID from their name; agents import their tools under it. Naming for local servers: not described.

## 8. Instructions and customization

- AGENTS.md: `AGENTS.md` is primary. `AGENT.md` and `CLAUDE.md` are fallbacks, used only when a directory has no `AGENTS.md`. [docs](https://ampcode.com/docs/customize/agents-md)
- Loaded from: the working directory and its parents up to `$HOME`; subdirectory files when the agent reads files there; `$HOME/.config/amp/AGENTS.md` and `$HOME/.config/AGENTS.md`; system-wide `/etc/ampcode/AGENTS.md` (macOS: `/Library/Application Support/ampcode/AGENTS.md`). Workspace guidance does not apply to subagents. [docs](https://ampcode.com/docs/customize/agents-md)
- Per-process control: `AMP_IGNORE_GUIDANCE_FILES` takes a colon-separated list of path globs to skip (`*` skips all). No flag or environment variable adds an instruction file. [docs](https://ampcode.com/docs/customize/agents-md)
- Per-process settings file: `--settings-file <path>` selects a custom user settings file (also `settingsFile` in the SDK). [docs](https://ampcode.com/docs/cli/settings), [docs](https://ampcode.com/manual/sdk/typescript)
- Modes: SDK `mode` takes `low`, `medium` (default), `high`, `ultra`, and plugin-defined mode strings. The CLI mode flag is not confirmed: the official CLI getting-started page was not readable. Third-party pages name other modes; not used. [docs](https://ampcode.com/manual/sdk/typescript)
- Tools: `amp.tools.disable` takes names, `builtin:` prefixed names, and `*` globs. SDK `enabledTools`. [docs](https://ampcode.com/docs/cli/settings), [docs](https://ampcode.com/manual/sdk/typescript)
- Permissions: by default Amp does not ask for approval before running tools. `amp.permissions` is an ordered list of rules (first match wins), each with `tool`, `matches`, and `action`: `allow`, `reject` (with a message), `ask`, or `delegate` (with `to`, a helper program). The delegate exit-code contract is not verified. [docs](https://ampcode.com/news/tool-level-permissions)
- Skills: `amp.skills.path` (extra directories), `amp.skills.disableClaudeCodeSkills`, `amp.skills.disableGlobalAgentsSkills`. [docs](https://ampcode.com/docs/cli/settings)
- Subagents: they emit a `system` `subagent_finished` message with status `completed`, `failed`, or `cancelled`. How to configure them: not checked.

## 9. Getting messages INTO a running session from outside

- Headless: `--stream-json-input` takes user messages as JSON lines on stdin. A line with `steer: true` queues guidance during a turn. Requires `--execute --stream-json`. [docs](https://ampcode.com/docs/cli/streaming-json)
- Plugins: `agent.start` appends text to the turn's user message. `agent.end` returns `continue` with a `userMessage` to start another turn. These are the only turn-boundary injection points documented. [docs](https://ampcode.com/docs/customize/plugins)
- Interactive TUI: typing into the PTY is the only route found. Whether another process can send messages to a running TUI: unknown. The threads page says a CLI thread "can be messaged from ampcode.com through Cross-Client Access", but the mechanism is not documented.
- Orbs: `amp threads continue T-… -ox '…'` sends a message to a thread running in an orb. [docs](https://ampcode.com/docs/threads)

## 10. Status detection from outside

- Headless: `system` `init` marks the start. `assistant` and `user` messages stream progress. `result` marks the end, with subtype `success` or an error subtype. `subagent_finished` marks subagent completion. No busy, idle, or waiting-for-permission event is documented. [docs](https://ampcode.com/docs/cli/streaming-json)
- Plugins: `agent.start` and `agent.end` bracket each turn. `tool.call` fires before each tool runs. A plugin could report status outward, if its API allows it (section 6). [docs](https://ampcode.com/docs/plugin-api)
- Permission waits: `ctx.ui.confirm` with `requireHuman` in plugins. Whether a headless run blocks, fails, or auto-declines on confirm: unknown.
- Terminal signals: none documented. Unknown.

## 11. Usage and cost reporting

- `usage` on `assistant` and `result` messages: `input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `cache_creation` (`ephemeral_5m_input_tokens`, `ephemeral_1h_input_tokens`), `output_tokens`, `max_tokens`, `service_tier`. The streaming page documents no cost field. [docs](https://ampcode.com/docs/cli/streaming-json)
- `result` on success: `duration_ms`, `duration_api_ms` (optional), `is_error`, `num_turns`, `result`, `session_id`, optional `usage` and `permission_denials`. Failure: `is_error: true` with `error`. [docs](https://ampcode.com/docs/cli/streaming-json)
- Cost display in the CLI: `amp.showCosts` (default true). [docs](https://ampcode.com/docs/cli/settings)
- Cost per message or per thread in machine output: not documented. Unknown.

## 12. Gotchas for running many instances side by side

- Scripts need `AMP_API_KEY` set to an `sgamp_` access token. The short-lived token from `amp login` is rejected in execute mode. [docs](https://ampcode.com/docs/cli/execute-mode)
- Commit trailers are on by default. `amp.git.commit.ampThread.enabled` (default true) adds a thread trailer; `AMP_DISABLE_AMP_THREAD_TRAILER=1` turns it off. `amp.git.commit.coauthor.enabled` (default true) adds Amp as a commit co-author, and only the settings key controls it. Set it to `false` for each process, through `--settings-file`, if your commit policy forbids attribution. [docs](https://ampcode.com/docs/cli/settings)
- Auto-update: `amp.updates.mode` defaults to `auto`. `AMP_SKIP_UPDATE_CHECK=1` disables all update checks. Set this on every instance to keep the agent version fixed. [docs](https://ampcode.com/docs/cli/settings)
- Plugin chaining: after five consecutive `agent.end` continues, Amp stops unless `maxContinuations` is set on the result. [docs](https://ampcode.com/docs/customize/plugins)
- `--plugin-ready-timeout` makes execute mode wait for plugins to load before the turn starts. The behavior without it is not stated: unknown. [docs](https://ampcode.com/docs/cli/execute-mode)
- Workspace AGENTS.md guidance does not reach subagents. [docs](https://ampcode.com/docs/customize/agents-md)
- Closed source: behavior can only be learned from docs and output, and the CLI auto-updates by default.
- Unknown, and each could break a design: exit codes; tool-permission behavior in execute mode; whether a stdin stream survives a `result`; per-thread storage location; concurrent instances in one directory.

## 13. Fit for Hypercube

- **Headless agent process:** `amp --execute --stream-json --stream-json-input` is the strongest candidate if it stays alive across turns, since the `steer` flag handles messages during a turn. Test that first. The SDK's `execute()` is the other option: a Node or Bun driver that spawns the CLI, with no PTY.
- **Conversation tracking:** learn `session_id` (`T-…`) from the first `system` `init` message. Resume with `threads continue T-…`. There is no documented way to choose the id at creation.
- **Per-process hooks:** point `XDG_CONFIG_HOME` at a private directory so the system plugin loads only Hypercube's plugin. The plugin's `agent.start`, `agent.end`, and `tool.call` events cover status and turn-boundary delivery. Test whether `XDG_CONFIG_HOME` also moves credentials.
- **Gaps:** no documented busy, idle, or permission-wait signal outside the plugin API; no caller-chosen ids; no documented per-process plugin file flag; closed source; no official ACP; interactive TUI behavior under xterm.js untested.
