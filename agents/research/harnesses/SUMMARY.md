# Harness research summary

Synthesis of the ten reports in this directory (2026-10-29), `~/projects/hypercube/docs/teamwork.md`, and `~/projects/hypercube/docs/harness-support.md` (how the Rust Hypercube launches and messages Claude Code and OpenCode today). Reports were not edited.

**Evidence tags.** `✓v` ran locally. `✓h` flag present in local `--help`, not exercised. `✓d` docs or source only. `~` partial or indirect. `✗` not found in docs or help (absence of evidence, not proof). `?` unknown. Installed locally: Claude Code, OpenCode, Codex, Copilot CLI, Pi. **Not installed, docs only: Gemini CLI, Aider, Goose, Amp, Cursor CLI.**

## 1. Capability matrix

### A. Identity and per-process wiring (can Hypercube set it up without touching the user's config?)

| Harness | Caller-chosen id | Resume by id | Per-process hooks/events | Per-process plugins | Per-process MCP | Per-process instructions |
|---|---|---|---|---|---|---|
| Claude Code | ✓v `--session-id <uuid>` | ✓d `--resume <id>` (drops per-process flags) | ✓v `--settings '{hooks}'` | ✓h `--plugin-dir` | ✓v `--mcp-config` | ✓h `--append-system-prompt` (snapshot caveat, §2) |
| OpenCode | ✓v `--session <id>`, created on first prompt | ✓v `--session` | ✓v inline `OPENCODE_CONFIG_CONTENT` + `--standalone` | ✓v same | ✓v same | ~ no flag; plugin `context` hook ✓d |
| Gemini CLI | ✗ | ✓d `-r <uuid>` | ✗ settings files only | ✗ `-e` selects installed ones | ✗ `--allowed-mcp-server-names` only filters | ✓d `GEMINI_SYSTEM_MD` (full replace) |
| Aider | ~ chat-history file path | ✓d `--restore-chat-history` | ✗ | ✗ | ✗ no MCP client | ✗ `--read` adds files only |
| Goose | ✗ `--session-id` requires `--resume`; `--name` is a label | ✓d `--resume --session-id` | ~ only via private `GOOSE_PATH_ROOT` | ~ same | ✓d `--with-extension` | ~ `--system` on `run`; `session`: ? |
| Amp | ✗ id `T-…` learned from `init` | ✓d `amp threads continue T-…` | ~ plugin events; private `XDG_CONFIG_HOME` | ~ same | ✓d `--mcp-config` (documented for execute mode) | ✗ |
| Codex | ? | ✓v `codex resume <id>` | ? `-c` untested; hook trust gate | ? | ? `-c mcp_servers` untested | ? |
| Copilot CLI | ✓h `--session-id <uuid>` | ✓h `--resume=<id>` | ~ `COPILOT_HOME` (also moves auth); `--plugin-dir` hooks ? | ✓h `--plugin-dir` | ✓h `--additional-mcp-config` | ✓h `COPILOT_CUSTOM_INSTRUCTIONS_DIRS` |
| Cursor CLI | ~ `agent create-chat` returns an id before launch ✓d | ✓d `--resume <chatId>` | ? `--plugin-dir` | ✓d `--plugin-dir` | ? via plugin `mcp.json` | ? plugin rules |
| Pi | ✓h `--session-id` | ✓h `--session` | ✓h `--extension` | ✓h `--extension` | ✓d built-in; extension `registerMcpServer` | ✓h `--append-system-prompt` |

### B. Runtime control (messages in, status out)

| Harness | Message injection into a running session | Status detection | ACP | Open source |
|---|---|---|---|---|
| Claude Code | Wake idle: ✓d `asyncRewake` hook waiter (used by the Rust Hypercube; **claude-code.md §9 missed it**). Boundary: ✓d `Stop` block, `PostToolUse` additionalContext. PTY. | Hooks ✓d (`UserPromptSubmit` busy, `PermissionRequest`/`Notification` needs-input, `Stop` idle; SessionStart per-process ✓v). `claude agents --json` busy/idle ✓v. OSC 7501 ✓d, bytes unknown. | adapter only (wraps Agent SDK) | ✗ proprietary |
| OpenCode | ✓d plugin `ctx.session.prompt` (steer/queue), `context` hook. In production use by the Rust Hypercube. | Events `session.execution.*`, `permission.asked` (names ✓v, payloads ?). `--auto` means no permission waits. | ✓v native `opencode acp` | ✓ MIT |
| Gemini CLI | Boundary only: `AfterAgent` deny/exit 2 ✓d. ACP `prompt` (no TUI). PTY. | Hooks `BeforeAgent`/`AfterAgent`/`Notification ToolPermission` ✓d (settings-file only). OSC 9 opt-in ✓d. | ✓d native `--acp` (flag name unresolved) | ✓ Apache-2.0 |
| Aider | PTY typing; `--watch-files` comments edit user files ✓d | Idle only: `--notifications-command` ✓d; busy and permission not separable | ✗ | ✓ Apache-2.0 |
| Goose | Boundary: `Stop` block, fixed wording ✓d. ACP `SteerSessionRequest` (no TUI). PTY. | OSC 7501 ✓d (format documented; `done` = idle; `blocked:permission\|question`). `GOOSE_STATUS_HOOK` undocumented. Hooks ✓d. | ✓d native `goose acp`/`serve` | ✓ Apache-2.0 |
| Amp | Boundary: plugin `agent.end` continue ✓d. `--stream-json-input` steer (headless, no TUI). PTY. | Plugin `agent.start`/`agent.end`/`tool.call` only; permission ? | ✗ official | ✗ closed |
| Codex | `codex queue --thread --message` ✓v exists, idle behavior ?. App-server `turn/steer` ✓d. `Stop` block ✓d. Async hooks do not wake idle ✓d. | Hooks incl. `PermissionRequest`, `Stop` ✓d. App-server `thread/status/changed` ✓d, only if the TUI runs against it. | ? | ✓ Apache-2.0 |
| Copilot CLI | ? PTY only known. ACP/SDK drive their own sessions. | Hooks partial ✓d (`userPromptSubmitted`, `agentStop`); permission ? | ✓h native `--acp` | ? binary closed or unstated; docs repo public |
| Cursor CLI | Boundary only: `postToolUse.additional_context`, `stop.followup_message` (loop cap 5) ✓d. No idle wake. | Hooks ✓d, no permission event in TUI. ACP `session/request_permission`. | ✓d native `agent acp` | ✗ |
| Pi | ✓d extension `pi.sendUserMessage`; idle wake needs a test | Extension events `agent_start`, `agent_settled` ✓d. No permission system. | ? | ✓ MIT (npm metadata ✓v) |

## 2. Cross-cutting findings

### What is universal

- Every harness has an interactive terminal mode and a headless mode. Aider and Goose are line REPLs; the rest are TUIs. Pi is full-screen by default (`--tui-mode regular` exists). Claude Code defaults to the main screen.
- All ten can resume a specific conversation. Aider resumes through a chat-history file.
- Nine of ten have lifecycle hooks or events (Aider has none), and the vocabulary converged on Claude's: session start, prompt submit, pre/post tool, stop, session end (Claude, Codex, Goose, Cursor; Gemini and Copilot rename them). `translate` is mostly a name and field mapping.
- `session_id` is in nearly every hook payload (Claude, Codex, Gemini, Goose, Cursor as `conversation_id`, Amp as `thread.id`). Learning a harness-assigned id from the first hook is the common path.
- A turn-end feedback path exists almost everywhere (Claude, Codex, Gemini, Goose, Cursor, Amp, OpenCode, Pi). Only Aider lacks one; Copilot is unknown.
- None of the ten has been tested under xterm.js.

### What varies

- **Per-process wiring has four tiers.**
  - Flag or inline config: Claude, OpenCode (`--standalone` only), Pi, Copilot (MCP, plugins).
  - Plugin directory flag: Copilot, Cursor. Hook coverage is untested.
  - Private config root: Goose (`GOOSE_PATH_ROOT`), Amp (`XDG_CONFIG_HOME`), Copilot (`COPILOT_HOME`). Credentials and sessions move with it, so auth must be provisioned. Side effects untested.
  - Settings files only: Gemini, Aider (nothing at all). Gemini's workspace `.gemini/settings.json` could work for worktree workers, but not for a lead that shares the main checkout. Inference, untested.
- **Who picks the conversation id.** Caller: Claude, OpenCode, Copilot, Pi. Caller-obtained: Cursor, Aider (file path). Harness: Gemini, Goose, Amp. Codex unknown. Never fall back to "latest" or `--continue`: Goose's bare `--resume` picks the most recent session across all directories, and Gemini's `-r latest` and Claude's `--continue` are ambiguous with several instances in one directory.
- **Resume does not restore wiring.** Claude drops `--settings`, `--mcp-config`, `--plugin-dir` on resume, so `launch` must rebuild everything each time.
- **Instructions go stale on resume.** Claude snapshots the system prompt on the first request and ignores a changed one on resume (`--system-prompt-snapshot`, default on). Keep `instructions` static and send dynamic team state through the SessionStart hook's context or the first message, as the Rust Hypercube does.
- **Harness-side retention.** Gemini deletes sessions older than 30 days by default (`general.sessionRetention`), set in the settings files. Long-lived sessions there are at risk.
- **Resource cost.** OpenCode needs `--standalone` for per-process config: about 147 MiB idle each (one sample), roughly 9-14 GiB at 60-100 agents. No other report measured memory.

### What is commonly missing

- **No harness offers a documented external API into a running TUI.** Every "API" (ACP, app-server, Pi RPC, Amp `--stream-json-input`, `opencode serve`, Goose `serve`) is a separate process with no TUI. The exceptions are in-process components the harness loads: Claude's `asyncRewake` hook, an OpenCode plugin, a Pi extension. `codex queue` is a possible external route (untested).
- **No caller-chosen id** for Gemini, Goose, Amp, and possibly Codex.
- **No per-process MCP** for Gemini and Aider. MCP is how agents call `send_message` and `list_agents` today, so a harness without it needs a CLI shim on PATH, or it cannot join a team.
- **Permission resolution is poorly signaled.** The wait is signaled in Claude, Codex, Gemini, OpenCode, Goose. The answer is not: Claude fires no hook on a denial, and the Rust Hypercube clears the flag from the status line. Cursor (TUI), Copilot and Amp have no documented permission-wait signal. Aider conflates it with idle. Pi has no permission system.

### Message delivery (what Hypercube's teamwork depends on)

The lead sits idle most of the time while workers run, and workers report to it. Delivery to an idle lead is the critical path. The user also types into the lead.

| Tier | Where it works | Limits |
|---|---|---|
| **Wake an idle session from inside** | Claude `asyncRewake` waiter ✓d; OpenCode plugin `session.prompt` ✓d; Pi extension `sendUserMessage` (untested) | The harness runs a Hypercube-supplied component that pulls from Hypercube. The typed draft stays in the editor (Claude, OpenCode per the Rust docs). Codex says async hooks do not wake idle. |
| **Feedback at a boundary** | Claude `Stop`/`PostToolUse`; Gemini `AfterAgent`; Codex `Stop`; Goose `Stop` (fixed wrapper text); Cursor `stop`/`postToolUse`; Amp `agent.end`; OpenCode events | Only fires when a turn ends or a tool finishes. Capped loops: Claude 8, Amp 5, Cursor 5, Goose unknown. |
| **External API to the TUI process** | `codex queue` (untested) | Rare. |
| **PTY typing** | Everything | Universal fallback. Risks: it overwrites or corrupts the user's draft, and **if a permission dialog is open the text answers it**. Gate on status, never type while needs-input or unknown. Bracketed paste plus submit was never tested on any harness. |

Harnesses with only PTY typing for the lead: Gemini, Aider, Goose (idle), Amp, Cursor, Copilot (unknown), Codex (unless `queue` works). For a boundary-only harness the lead can still be reached when a turn ends, which is enough for workers that report while the lead is mid-turn, not for an idle lead.

### Status detection

| Source | Harnesses | Notes |
|---|---|---|
| Per-process hooks | Claude, OpenCode (plugin), Pi (extension) | Full busy/idle/permission coverage in Claude and OpenCode. |
| Settings-file hooks | Gemini, Goose, Amp, Cursor, Copilot, Codex | Per-process only with a private root or a plugin-dir route. |
| **OSC in the PTY stream** | Claude (OSC 7501, ≥2.1.295, not in tmux, bytes unknown ✓d); Goose (OSC 7501, format documented ✓d); Gemini (OSC 9, opt-in); Goose/Aider BEL (opt-in) | Needs no hook wiring, works for any process whose output Hypercube already parses (the server has a headless xterm per terminal). Goose reports "waiting for input" as `done`, so treat `done` as idle and `idle` as cancelled. |
| Poll a harness command | Claude `agents --json` busy/idle ✓v | No blocked state seen for interactive sessions. |
| Notification command | Aider (idle only) | Gets no env, so the agent id goes in the command string. |
| Server/ACP events | Codex app-server, OpenCode, Goose, Gemini, Cursor, Copilot | Only with a non-PTY process. |

Gaps: busy for Gemini and Aider without hooks, permission for Cursor TUI, Copilot and Amp, and a "permission resolved" signal everywhere.

## 3. Implications for the plugin interface

### Proposed interface

```ts
interface HarnessPlugin {
  id: string; label: string;
  kind: "pty";                                      // 7: reserve the discriminant, see ACP below
  capabilities: {
    resume: boolean;
    conversationId: "caller" | "harness";           // 2
    conversationEvents: boolean;
    activity: { busy: boolean; permission: boolean; sources: ("hook" | "osc" | "notify")[] };  // 3
    delivery: { boundary: boolean; wake: "native" | "none" };                                 // 1
    mcp: "per-process" | "shared-config" | "none";
    instrumentation: "flag" | "private-root" | "project-file" | "none";                       // 5
    resources?: { idleRssMiB?: number };            // 6
  };
  check(): Promise<{ ok: true; version?: string; warnings?: string[] } | { ok: false; reason: string }>;
  newConversationId?(): string | Promise<string>;   // 2: absent means the harness assigns it
  setup(dir: string, agent: AgentView): void;       // 5: per agent, not per install
  launch(ctx: LaunchContext): { command: string[]; env: Record<string, string> };
  parse(raw: unknown, agent: AgentView): HarnessEvent[];                                       // 4
  respond?(raw: unknown, out: { inject?: string; block?: string }): unknown;                   // 4
  parsePty?(osc: { code: number; data: string }, agent: AgentView): HarnessEvent[];            // 3
  deliver?(agent: AgentView, text: string, io: { writePty(b: string): void }): Promise<"sent" | "unsupported">;  // 1
}
type LaunchContext = {
  cwd; conversationId; resume: boolean;             // 2
  role: "lead" | "worker"; harnessState;
  report: { url; token }; instructions?; mcp?: { url; token };
  prompt?; model?; effort?;                         // 2
};
type HarnessEvent =
  | { type: "conversation"; id; cause: "start" | "resume" | "new" | "switch" }
  | { type: "activity"; state: "busy" | "idle" | "needs-input"; detail?: "permission" | "question" }
  | { type: "state"; harnessState };
```

### Changes, most important first

1. **Message delivery is a plugin concern, with the queue owned by core.** Core holds each agent's queue. `deliver` is optional, and the default is PTY typing. `capabilities.delivery` declares what the harness can do. Core picks per message: hook reply at a boundary, then native wake, then PTY typing, in that order. Rules for core:
   - Never type into the PTY unless activity is `idle`. A permission dialog would consume the text, and a busy TUI may queue or lose it.
   - Prefer native wake for the lead, which is idle most of the time and where the user types.
   - Experiment: bracketed paste plus submit per harness, and behavior with a half-typed draft.
2. **The hook path needs a reply, not just events.** `translate` as drafted is one-way. In Claude, OpenCode, Codex, Gemini, Goose, Cursor and Amp the delivery channel is the hook's response (`additionalContext`, a `Stop` block reason, `followup_message`, `agent.end` continue). Split into `parse(raw) → events` and `respond(raw, { inject?, block? }) → harness-shaped body`. The reply text comes from core's queue, which keeps the queue in one place. Claude also documents an `http` hook type, which would POST straight to the endpoint with no shell script (auth header support unverified).
3. **Status from the PTY stream, not only hooks.** Add `parsePty` and register OSC handlers on the headless xterm that already parses every terminal. A core-level OSC 7501 decoder covers Claude and Goose (the second a documented format). OSC 9 and BEL are lower-tier hints. Split `activityEvents: boolean` into `busy` and `permission` coverage plus `sources`, so a harness with idle-only (Aider) or no permission system (Pi) is explicit. Core rule: needs-input clears on any later event (busy, prompt, tool result, OSC `working`), because denial fires no hook in Claude. Client xterm handling of unknown OSC is untested; the PTY output reaching the browser is unchanged.
4. **Conversation id: make caller-chosen optional and add `resume` and `prompt` to the launch context.**
   - `newConversationId?` is optional and may be async (Cursor needs `create-chat`). When absent, core waits for a `conversation` event (`cause: "start"`) from the first hook and stores it. Gemini, Goose, Amp and probably Codex need this.
   - `resume: boolean`, so Claude gets `--session-id` for a new conversation and `--resume` for an existing one. OpenCode creates the session lazily, so "exists" is known only after the first prompt; record it from the event, not at launch.
   - `launch` must be a pure function of `ctx` and rebuild every per-process flag each time, since Claude's `--resume` drops them.
   - Never pass "latest", `--continue` or a bare `--resume`.
   - `cause: "new"` / `"switch"` must be honored mid-run: `/clear` and `/resume` inside a TUI change the id.
   - `prompt`, `model`, `effort` are needed because the Rust Hypercube passes them (workers start with `Check your Hypercube messages.`).
5. **Declare how instrumentation reaches the process, and write it per agent.** `capabilities.instrumentation` is one of `flag` (Claude, Pi, Copilot, OpenCode), `private-root` (Goose, Amp, Copilot), `project-file` (Gemini, Cursor: written into the agent's cwd, safe only for worktree workers, which already live under `<checkout>/.hypercube/<name>`), `none` (Aider). `setup(dir, agent)` becomes per agent so a private root or project file has an owner. `check()` gains `warnings` for things like "private root also isolates credentials". `none` plus `mcp: "none"` should block team membership, or fall back to a CLI shim.
6. **Per-instance cost is a hint, not a limit.** Add optional `resources.idleRssMiB` so core can warn or cap live agents per harness. OpenCode `--standalone` at about 147 MiB × 60-100 agents is the one known pressure. Attaching many TUIs to one shared server would cut that, but per-process config only reaches a private server, so it is a trade-off. Measure idle RSS for each harness before choosing numbers.
7. **ACP and server modes: out of scope for v1, keep the door open.** Five harnesses have native ACP (OpenCode, Gemini, Goose, Copilot, Cursor), and Claude has an SDK adapter, and it gives structured status, explicit permission requests and steering. But the process has no TUI, so the user cannot watch or type into it, and Hypercube's terminal model (PTY ownership, snapshots, resize) does not apply. A later `kind: "acp"` plugin can reuse `HarnessEvent` unchanged. The `kind: "pty"` field costs nothing now. One hybrid is worth an experiment: a TUI attached to a Hypercube-owned server (OpenCode `attach`, Codex `--remote`), which would give both a terminal and an API.
8. **Small:** do not add an `ended` event. Core owns the PTY exit. A usage/cost event can wait; the sources differ too much to normalize yet (Claude statusLine and `--output-format json`, OpenCode `stats`, Goose `complete`, Copilot `--usage-output-file`).

## 4. Recommended first harnesses after the dummy one

1. **Claude Code**: the only harness with an id, hooks, MCP, instructions and idle wake all per-process and mostly verified, and it matches what the Rust Hypercube already does.
2. **OpenCode**: already supported in the Rust version, and a plugin gives the strongest in-process delivery and events; budget for the standalone memory cost.
3. **Pi**: every per-process surface is available as a flag, and an extension can register MCP, relay lifecycle and inject messages. It is the cleanest test that the interface does not assume Claude's shape. Needs an idle-wake test and a full-screen check.
4. **Codex**: installed, open source, Claude-shaped hooks including `PermissionRequest`, and `queue` may give external delivery. Gate on the experiments for per-process hooks and MCP via `-c`.
5. **Copilot CLI**: caller id, plugin dir, MCP and instructions are all per-process flags. Gate on whether hooks load from `--plugin-dir` and on a permission signal.

Defer: **Goose** (docs only, no caller id, 2.0 rewrite may change the CLI and OSC output; revisit for its OSC 7501 status), **Cursor CLI** (closed, plugin-dir path unproven), **Gemini** (no per-process hooks or MCP), **Amp** (closed, no per-process plugin flag), **Aider** (no hooks, no MCP, idle-only status).

## 5. Needs a local experiment before we rely on it

| # | Experiment | Why |
|---|---|---|
| 1 | Claude `asyncRewake` waiter wakes an idle session on 2.1.312, from `--settings` | The report called idle wake impossible; the Rust Hypercube depends on it. Also check the `http` hook type and its auth. |
| 2 | Capture Claude's OSC 7501 bytes in our PTY (env needed to enable, tmux exclusion), and confirm the headless xterm and the browser xterm tolerate it | Report could not find the format. Goose's format is documented only. |
| 3 | PTY typing delivery on each harness: bracketed paste, submit, draft in the editor, dialog open | Fallback for the whole matrix; never exercised. |
| 4 | Codex: per-process hooks and MCP via `-c`, hook trust gate, `codex queue` into an idle TUI, `--remote` | Four unknowns decide whether it is tier 1 or 2. |
| 5 | OpenCode: `permission.asked` payload and whether `--auto` suppresses it, real route table on 2.2.0, RSS with a real TUI, concurrent SQLite writers | Docs routes did not answer on 2.2.0. |
| 6 | Pi: `sendUserMessage` waking an idle session, an extension waiting on an external signal, full-screen rendering in xterm.js | The in-process delivery claim is docs-only. |
| 7 | Copilot: hooks from `--plugin-dir`, permission-wait signal, shared `COPILOT_HOME` contention | Decides whether per-process status is possible. |
| 8 | Cursor: do `--plugin-dir` hooks and MCP load in the CLI | Docs-only. |
| 9 | All five uninstalled harnesses: install in a scratch environment and re-check every `✓d` | Nothing about them was run. |
| 10 | Idle RSS per harness, and a 60-100 process load test with a mix | Only OpenCode has a number, from one sample. |
| 11 | xterm.js behavior and redraw-after-attach for every TUI (Pi, Copilot, OpenCode, Codex) and the line REPLs (Aider, Goose) | Invariant 5 assumes an app redraws on resize or on request; a line REPL has nothing to redraw. |
