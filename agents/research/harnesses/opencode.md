# OpenCode

Researched 2026-10-29. Installed locally: `opencode` v2.2.0 (`/opt/homebrew/bin/opencode` → `Cellar/opencode-v2/2.2.0`).

Tags:
- **[verified]**: run here on 2.2.0.
- **[docs]**: read from the linked page. The pages were read through a summarizing fetch, so names are as extracted, not byte-checked. Re-check exact spellings before building on them.
- **unknown**: not established.

The v2 docs live under `opencode.ai/v2/docs/...`. The server, CLI and config pages under `opencode.ai/docs/...` read as v1-era; where they disagree with 2.2.0, this file says so.

## 1. Overview

- What: open-source AI coding agent with an interactive TUI, a `run` mode, a server, an ACP server, and a plugin API. [docs](https://github.com/anomalyco/opencode)
- Vendor: GitHub org `anomalyco`. Homebrew tap `anomalyco/tap` [verified: install receipt].
- License: MIT [docs](https://github.com/anomalyco/opencode).
- Version: 2.2.0 [verified: `opencode --version`]. Tap stable is 2.2.0 [verified: install receipt].
- Install: `brew install anomalyco/tap/opencode-v2` (listed as recommended), `curl -fsSL https://opencode.ai/v2/install | bash`, `npm i -g @opencode/cli@latest` [docs](https://github.com/anomalyco/opencode). Installed here via the brew formula `opencode-v2` [verified: receipt, `installed_on_request`]. A v1 formula `brew install opencode` also exists [docs](https://github.com/anomalyco/opencode).
- Adoption: 223k GitHub stars at fetch time [docs](https://github.com/anomalyco/opencode).

## 2. Interactive mode in a PTY

- `opencode` with no subcommand starts the TUI [docs](https://opencode.ai/docs/cli/).
- The TUI plugin API exposes sidebar, footer and dialog slots [docs](https://opencode.ai/v2/docs/build/plugins/cli). I read that as a full-screen app; the docs do not say so.
- Top-level flags: `--standalone`, `--server <url>`, `--auto`, `--continue/-c`, `--session/-s`, `--prompt` [verified: `opencode --help`].
- Behavior under xterm.js: unknown. I did not start the TUI. It would touch the shared DB and service, and no non-model command exercises it. To settle it, run `opencode --standalone <dir>` in our own PTY with `OPENCODE_DB` pointed at a temp file. The variable name exists in the binary [verified]; its behavior is untested.
- Resize behavior: unknown.
- `opencode attach <url>` connects a TUI to a running server [docs](https://opencode.ai/docs/cli/).

## 3. Headless, server, and ACP modes

- `opencode run [message...]` [verified: `run --help`]. Flags: `--format default|json`, `--session/-s`, `--continue/-c`, `--fork`, `--model/-m provider/model#variant`, `--agent`, `--file/-f`, `--title`, `--thinking`, `--auto`. Global flags include `--standalone` and `--server`.
  - `--format json` emits "raw JSON events" [docs](https://opencode.ai/docs/cli/). Event names and schema: unknown. I made no model call.
  - `--attach` is in the docs [docs](https://opencode.ai/docs/cli/) but not in 2.2.0's `run --help` [verified].
  - Whether `run` exits when the turn ends: unknown.
- `opencode serve [--hostname] [--port] [--cors] [--service] [--stdio]` [verified: `serve --help`]. The v1 server page lists `--mdns` and default port 4096 [docs](https://opencode.ai/docs/server/). `--mdns` is absent from 2.2.0's help [verified]. The 2.2.0 help shows no defaults.
- HTTP API. The v1 server page documents `POST /session`, `POST /session/:id/prompt_async`, `GET /event`, `POST /session/:id/permissions/:permissionID` and `/tui/*` [docs](https://opencode.ai/docs/server/). On a private 2.2.0 server, `GET /global/health`, `/mcp`, `/config` and `/session/status` each returned the web app's HTML, not JSON [verified]. So the documented routes did not answer on 2.2.0. The real 2.2.0 route table: unknown. `opencode api <operationId | METHOD path>` calls the running server by OpenAPI operation ID [verified: help]. The docs say the spec is at `/doc` [docs](https://opencode.ai/docs/server/). I did not test `/doc`.
- Auth: `OPENCODE_SERVER_PASSWORD` enables basic auth. `OPENCODE_SERVER_USERNAME` defaults to `opencode` [docs](https://opencode.ai/docs/server/). With the password variable unset, `serve` generated a password and wrote it to its output [verified]. The value is not recorded here.
- ACP: native. `opencode acp` starts "an ACP-compatible subprocess that communicates with your editor over JSON-RPC via stdio" [verified: subcommand exists; docs](https://opencode.ai/docs/acp/). The docs list built-in tools, custom tools and commands, MCP servers from config, `AGENTS.md`, formatters, and agents and permissions. `/undo` and `/redo` are unsupported [docs](https://opencode.ai/docs/acp/). Session load/resume and modes: not documented, so unknown. `--login` runs auth login [verified].

## 4. Conversation identity

- Caller-chosen ID: `--session <id>`, described as "Session ID to continue, or to create if it does not exist" [verified: help]. `opencode session list` shows rows with IDs of the form `ses_hypercube_<8 hex>`, created by Hypercube agents [verified]. So caller-chosen `ses_` IDs are accepted in practice.
- When the session row is created, at launch or at the first prompt: unknown.
- Learning the ID back: not needed when the caller picks it. `opencode session list` shows top-level sessions in the current project, newest first [verified: help].
- Resume: `--session <id>`, or `--continue` for the last session. `--fork` forks on continue [verified: help].
- Storage: SQLite at `~/.local/share/opencode/opencode.db`, with `-wal` and `-shm` files [verified: `opencode debug paths`, `ls`]. The DB is 1.5 GB at the time of research. The data dir also has `log/`, `snapshot/`, `tool-output/` and `repos/` [verified: `ls`]. `opencode session export|import` moves session JSON [verified: help].
- Several instances in one directory: sessions are scoped to the project and keyed by ID [docs: `session list` is per project; verified in help]. Two session IDs in one directory should not collide at the session level; that is inferred, not tested. Concurrent writes from several processes to one SQLite file: unknown, untested.

## 5. Hooks and events

Server plugin hooks, registered as `ctx.<domain>.hook(name, callback)` [docs](https://opencode.ai/v2/docs/build/plugins/server):

- `ctx.session.hook("prompt")`: runs once at prompt admission, not before each model call. Input: `prompt` (`text`, `files`, `agents`, `skills`), `metadata`, and `delivery` (`"steer"` by default, or `"queue"`).
- `ctx.session.hook("context")`: runs on every agent-loop step, including tool continuations. Fields: `system`, `messages`, `tools`, `options`, `agent`, `model`, `sessionID`.
- `ctx.session.hook("compaction")`, `"title"`, `"retry"`, `"model.request"`, `"http.request"`, `"http.response"`: documented [docs]; not needed for Hypercube.
- `ctx.tool.hook("execute.before")` and `ctx.tool.hook("execute.after")` [docs]. `execute.after` receives `status` `"completed"` (writable `result`) or `"error"` (`error.message`). The name `execute.after` is in the binary [verified].
- `ctx.permission.hook("evaluate")`: `effect` is writable as `allow`, `ask` or `deny`. An explicit configured deny is final [docs].
- `ctx.shell.hook("create.before")`: `command`, `cwd`, `timeout`, `shell`, `env` are writable before a command runs [docs].

Events, via `ctx.event.subscribe({ types?, signal? })` [docs](https://opencode.ai/v2/docs/build/plugins/server):

- Turn finished: `session.execution.succeeded`, `session.execution.failed`, `session.execution.interrupted`. [verified in binary; the TUI docs use `session.execution.succeeded` as an example [docs](https://opencode.ai/v2/docs/build/plugins/cli)]
- Waiting for permission: `permission.asked`; resolved: `permission.replied`. [verified in binary; `permission.asked` is an example in the TUI docs]
- Other names in the binary [verified]: `session.status`, `session.idle`, `session.execution.started`, `session.prompt`, `config.updated`, `server.connected`. Per the docs, plugins never receive `server.connected`, which is SSE-only.
- Payload fields for these events: unknown.

Lifecycle mapping:

- SessionStart: no dedicated event documented. A server plugin first sees the session at the `prompt` hook [docs].
- Prompt submitted: `prompt` hook [docs].
- Tool use: `execute.before` / `execute.after` [docs].
- Turn finished: `session.execution.*` events [verified names].
- Waiting for a permission: `permission.asked` [verified name].
- Session end: none documented. `setup(ctx)` may return a cleanup function that runs at unload [docs].
- A turn-end hook that blocks the turn and feeds text back: none documented. The route the docs support is to react to a `session.execution.*` event and call `ctx.session.prompt` to start a new turn [docs].

Per-process injection: yes, without editing the user's config files.

- `OPENCODE_CONFIG_CONTENT` is inline config. In the precedence list it is step 6 of 8: above project config, below managed config [docs](https://opencode.ai/docs/config/).
- On 2.2.0 under `opencode --standalone`: an inline `agent.build.description` override took effect [verified]. An inline `mcp` local entry launched a process [verified; see §7]. Both `plugin` and `plugins` keys triggered a load attempt for a missing file [verified in debug logs].
- Without `--standalone`, `opencode mcp list` did not show an inline MCP entry [verified]. Per-process config therefore needs a private server.

## 6. Plugins and extensions

- Server plugin: `export default Plugin.define({ id, async setup(ctx) { ... } })` [docs](https://opencode.ai/v2/docs/build/plugins/server). `setup` may return a cleanup function.
- `ctx` provides `app.version`, `location` (`directory`, `workspaceID?`, `project`), `options`, durable `storage` (`get`, `set`, `remove`, `scan`), and domains for agents, providers, models, commands, tools, MCP, sessions, permissions, events, shell, VCS, worktrees and skills [docs].
- Plugins can add tools (`ctx.tool` editor `add`) and slash commands (`ctx.command` editor `add`). They can call `ctx.session.prompt`, `ctx.session.wait`, `ctx.session.interrupt` and `ctx.permission.reply` [docs].
- TUI plugin: import from `@opencode/plugin/tui`, `Plugin.define({ id, setup(context) })`, exported as `./tui` [docs](https://opencode.ai/v2/docs/build/plugins/cli). It has toasts, dialogs, slots, keymap and `attention.notify` [docs].
- Loading: a `plugins` array (v2 page) or a `plugin` array (v1 config page) in `opencode.json(c)` [docs]. Both keys trigger a load on 2.2.0 [verified]. Local plugin files in `.opencode/plugins/` or `~/.config/opencode/plugins/` load automatically [docs]. The global plugins dir exists and is empty [verified].
- `opencode plugin list|add|check|update|remove` [verified: help]. `add` writes to the global config, so avoid it. `opencode plugin list` here reports "No plugins found" [verified].
- V1 compatibility: a default export can spread V2 `Plugin.define` and add `server()` returning V1 hooks. V2 ignores `server()`, and the two APIs do not translate into each other. The object form needs OpenCode 1.18.29 or later [docs].
- Per-process loading: yes, via inline config (§5) [verified].

## 7. MCP

- Client: yes. The `mcp` config key is an object keyed by server name [docs](https://opencode.ai/docs/config/). The documented example is `type: "remote"` with `url` and `enabled`.
- Local stdio entries: `{"type":"local","command":[...]}` launched a process on 2.2.0 through inline config. The process exited with code 0 and was logged as a connection failure [verified].
- Remote transport details (HTTP vs SSE): unknown.
- Per-process MCP: yes, via `OPENCODE_CONFIG_CONTENT`, against a private server [verified].
- CLI: `opencode mcp list|add|auth|logout` [verified: help]. `mcp list` reads the shared service's config.
- ACP mode uses the MCP servers in the config [docs](https://opencode.ai/docs/acp/).
- The MCP status route `GET /mcp` did not answer on 2.2.0 (HTML) [verified].

## 8. Instructions and customization

- Per-process system prompt: no CLI flag in `run --help` or top-level help [verified]. A plugin `context` hook can rewrite `system` on each step [docs]. Config-level instructions: unknown.
- `AGENTS.md` is read in ACP mode [docs](https://opencode.ai/docs/acp/). Not checked in the TUI.
- Custom commands: `ctx.command.add({ name, description, execute })` [docs]. `.opencode` directories hold agents, commands and plugins [docs](https://opencode.ai/docs/config/).
- Agents: `--agent <name>` [verified: help]. `opencode debug agents` lists `build` (primary, the default) and `compaction` (hidden) [verified].
- Subagents: the `stats` JSON has a `subagents` count [verified]. `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` is documented [docs](https://opencode.ai/docs/config/). Details: unknown.
- Model and provider: `--model provider/model#variant` [verified: `run --help`]. `--variant` is documented [docs](https://opencode.ai/docs/cli/). Providers live in `opencode.jsonc`; the user's global file overrides some OpenRouter models [verified: `opencode debug config`]. `opencode models` lists models [verified: help].
- Permission modes: `permission` is an object keyed by tool, with values `ask`, `allow` or `deny`, and wildcard and per-pattern rules [docs](https://opencode.ai/docs/config/). `--auto` means "Auto-approve permissions that are not explicitly denied" [verified]. The default `build` agent allows `*`, asks for reads of `*.env`, and denies `browser` [verified: `opencode debug agents`].

## 9. Getting messages into a running session

- Plugin: `ctx.session.prompt` with `delivery` `"steer"` or `"queue"` [docs](https://opencode.ai/v2/docs/build/plugins/server).
- Plugin: the `context` hook can add messages or system text to the next model call, including tool continuations [docs]. That injects text without a new turn.
- Plugin: `ctx.session.wait({ sessionID })` and `ctx.session.interrupt({ sessionID, resume })` [docs].
- HTTP: `POST /session/:id/prompt_async` and `POST /tui/append-prompt` / `/tui/submit-prompt` [docs](https://opencode.ai/docs/server/). These are v1-era; the 2.2.0 routes did not answer in my probe (§3).
- `opencode run --attach` [docs](https://opencode.ai/docs/cli/). The flag is absent from 2.2.0's help [verified].
- Typing into the PTY: unknown, untested.

## 10. Status detection from outside

- Events: `session.status`, `session.execution.started` (busy), `session.execution.succeeded|failed|interrupted` (finished), `session.idle`, and `permission.asked` (waiting for permission). Names verified in the binary; payloads unknown.
- HTTP: `GET /session/status` [docs](https://opencode.ai/docs/server/), v1-era. Not confirmed on 2.2.0.
- Under `--auto`, unknown whether `permission.asked` still fires for auto-approved calls. This needs a test.
- `opencode stats` gives history, not live status [verified].

## 11. Usage and cost

- `opencode stats` flags: `--days`, `--year`, `--all`, `--project <id>|.`, `--models`, `--tools`, `--cost`, `--full`, `--limit`, `--json` [verified: help].
- `--json` keys [verified]: `range`, `sessions`, `subagents`, `prompts`, `steps`, `tokens{input, output, reasoning, cache{read, write}}`, `cost`, `tools{mode, totals, usage[]}`. Over the last day, for the whole machine: 9 sessions, 14 prompts, 190 steps, cost 0.1268 (currency not stated).
- Per-session cost: `ctx.data.session.cost` in the TUI plugin API [docs](https://opencode.ai/v2/docs/build/plugins/cli). The server-plugin equivalent: unknown.

## 12. Gotchas

- Shared service vs `--standalone`: without `--standalone`, commands use a shared background server. `opencode service status` printed `http://127.0.0.1:49374`, and the service's PID is 44025 [verified]. `--standalone` runs a private server [verified: help text]. Per-process config only reached the private server (§5, §7).
- Service state file: `~/.local/state/opencode/service.json`, mode 600, keys `id`, `version`, `url`, `pid`, `password` [verified keys and mode; value not read]. The password is stored in plaintext there.
- Memory: a private server idled at about 147 MiB RSS and was listening about 1 s after start [verified, one sample]. The shared service had about 390 MiB RSS after about 2.5 h [verified]. At 60–100 agents, idle private servers alone would be about 9–14 GiB. That is arithmetic from one sample, not a measurement at scale.
- Secrets in output: `serve` with `OPENCODE_SERVER_PASSWORD` unset wrote a generated password to its output [verified]. Keep server output out of logs.
- One SQLite DB (1.5 GB, WAL mode) holds everything [verified]. Multi-process write behavior: unknown.
- Docs drift: the v1 server, CLI and config pages describe flags and routes that 2.2.0 does not have (`--mdns`, `run --attach`, the route set) [verified via help and probe]. Use `--help` and the binary as the reference for flags.
- `plugin` vs `plugins`: the docs differ between v1 and v2. Both load on 2.2.0 [verified].
- Managed config: per the docs, managed files outrank user and inline config [docs](https://opencode.ai/docs/config/). `/Library/Application Support/opencode` does not exist on this Mac [verified]. On a managed machine, that could override Hypercube's inline config.

## 13. Fit for Hypercube

- Launch: `opencode --standalone --auto --session ses_hypercube_<id> --prompt <text>` in the agent's directory, with `OPENCODE_CONFIG_CONTENT` carrying the model, MCP and plugin. Each part was verified on a private server; the combined launch was not run.
- Conversation: the caller-chosen session ID is the resume key. Sessions persist in SQLite, and no ID read-back is needed.
- Status: the plugin subscribes to `session.execution.*` and `permission.asked`. The payload shapes need a one-time check on a throwaway session.
- Message delivery: `ctx.session.prompt` (queue) and the `context` hook. This is the strongest injection path of the two harnesses studied, and it needs no PTY typing.
- Gaps: per-agent memory at 60–100 agents; the 2.2.0 HTTP routes (the docs' routes did not answer); the `--format json` schema; TUI behavior in a PTY; concurrent SQLite writes; whether `--auto` suppresses `permission.asked`.
