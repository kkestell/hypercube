# Sessions: workspaces, leads, lifecycle, resume (built)

What this phase built: the server-owned model, UI flows, agent lifecycle, resume and the workspace shell. Conversation switching (/clear, /new, /resume, the OpenCode picker) is a separate, unbuilt plan: `conversation-split.md`.

## Invariants (AGENTS.md 1–7 still hold)

S1. **Server owns the model.** Workspaces, sessions, agents and shells live in server memory on `globalThis` (they survive page and `bun --hot` reloads; a server restart empties them). The browser renders the pushed model. It never spawns or closes a terminal, and it attaches only to terminal ids the model lists.
S2. **Terminal ids are never reused.** The server generates `t-<uuid>` per process. An agent's terminal id changes whenever its process is replaced. A stale id only gets `gone`.
S3. **Spawns come only from model operations:** add workspace (its shell), create session, start session, retry agent, restart shell, and auto-restart of a lead in a running session. A socket event, a page reload or a hook request never spawns a process. Once `shuttingDown` is set, nothing spawns.
S4. **At most one live process per agent.** A replacement is spawned only after the old terminal is gone: its session was reaped (the `ended` hook) or `closeTerminal` resolved.
S5. **Every model transition is one synchronous handler with no `await` inside.** `Bun.spawn` and `existsSync` are synchronous. Anything slow (a close finishing, an exit, a timer, a hook request) comes back later as an event. The event looks up the terminal's owner and the agent's state when it runs and does nothing if they no longer match. There are no queues; JS runs one handler at a time.
S6. **Agent auth is per process.** Every agent process gets a random 32-byte token via env, mapped to its terminal id. A token whose terminal is no longer the agent's is rejected.
S7. **A failed operation changes nothing.** Validation and spawning happen before the model is changed, or are rolled back in the same handler.
S8. **Workspace paths are canonical:** `realpath` after `~` expansion. A path is a duplicate if its realpath equals an open workspace's realpath.

## Model (`src/model.ts`, shared)

```ts
type Harness = "claude" | "opencode";
type Workspace = { id; name; path; nextSessionNumber; sessionIds: string[]; shell: Shell };
type Shell = { terminalId: string | null; exitCode: number | null };           // no auto-restart
type Session = { id; workspaceId; name; state: "running" | "stopped"; agentIds: string[]; createdAt };
type Agent = { id; sessionId; role: "lead" | "worker"; harness; name; conversationId; state: AgentState; terminalId: string | null; failure: string | null };
type AgentState = "running" | "restarting" | "stopping" | "stopped" | "failed";
type Model = { version; workspaces: Workspace[]; sessions: Record<string, Session>; agents: Record<string, Agent> };
```
Server-only per agent (`src/server/sessions.ts`, `Runtime`): `token`, `transcriptPath`, `startedAt`, `quickExits`, `retryTimer`, `adopted` (the live process has reported its conversation), `lastSeq` (highest OpenCode route sequence accepted). There are also maps for token → terminal and terminal → owner (agent or workspace shell).
Names: workspace = basename of the realpath. Session = `Untitled ${nextSessionNumber++}` (counts up, never reused). Lead name = harness label. Workers aren't created yet, but the model has them: workers are never restarted automatically.

## Agent state machine

| from | event | guard | to | action |
|---|---|---|---|---|
| stopped / failed | start (session start, retry) | not shutting down | running | spawn (resume rule below) |
| running | ended(code) | lead, session running | restarting | quick (<10s alive) → `quickExits++`, else 0; backoff 0.5s·2^(quickExits−1), max 4s |
| running | ended | lead, `quickExits` reaches 3 | failed | `failure = "Exited N times in a row (last exit code C)"` |
| running | ended | worker, or session stopped | stopped | — |
| restarting | timer | session running | running | spawn |
| restarting | stop | — | stopped | clear timer |
| running | stop | — | stopping → stopped | revoke token, `closeTerminal`; the `closed` event clears `terminalId` |
| stopping | closed | session running again | running | spawn (start arrived while stopping) |
| failed | stop | — | stopped | — |
| any | spawn throws (missing binary, deleted cwd) | — | failed | `failure = "Could not start: …"`, token revoked; no automatic retry |

Events: `closed(agent, T)`, `ended(T)`, `retry(agent)`, plus the hook requests. Each checks `agent.terminalId == T` (or its state) and is ignored otherwise. They call through `state.ops`, so a hot reload during a backoff or an awaited close runs the current code.

Registry owner hooks (`setOwnerHooks`): `ended(entry)` fires from `forget()` (the exit was published and the session reaped), never from `closeTerminal`. `beforeShutdown()` runs first in `registry.ops.shutdown`; it sets `shuttingDown` and clears every retry timer.

Rollback: `addWorkspace` validates, spawns the shell and only then inserts the workspace. `createSession` spawns the lead before it inserts anything or consumes a number. `restartShell` while a shell is live does nothing.

## Spawning and resume (`src/server/harness.ts`)

Cwd = the workspace realpath. Env adds `HYPERCUBE_URL=http://localhost:<port>` and `HYPERCUBE_TOKEN`. Every terminal drops the markers of a parent Claude Code session (`CLAUDECODE`, `CLAUDE_CODE_CHILD_SESSION`, …, list in `terminals.ts`). If the server was started inside Claude Code, an agent that inherits them stops saving transcripts, which breaks resume (verified).

- **claude**: `--session-id <id>` if there is no transcript, else `--resume <id>`; always `--settings <harnessDir>/claude-settings.json`. Never `--continue`. Transcript = `transcriptPath` reported by the SessionStart hook, if it exists on disk. Without a reported path (hooks disabled), the check is whether `~/.claude/projects/*/<id>.jsonl` exists (verified location; `$CLAUDE_CONFIG_DIR` respected). A fresh conversation has no transcript until something is recorded (verified).
- **opencode**: `--standalone --session <id>` (`id = ses_hypercube_<32 hex>`), env `OPENCODE_CLI_CONFIG_CONTENT` loading the plugin dir. Verified by ps on a manual run that the private `opencode serve --stdio` child is in the PTY's session and dies on close.
- `HYPERCUBE_CLAUDE_BIN` / `HYPERCUBE_OPENCODE_BIN` override the binaries (tests use `test/fixtures/fake-*`). Shell: `$SHELL -l`.

`harnessDir` = `$HYPERCUBE_HARNESS_DIR` or `~/.cache/hypercube/harness/`. It is rewritten at every load, each file atomically (temp file + rename). The contents are the same for every server instance; anything per process comes through env. Files: `claude-hook.sh` (curl POSTs the SessionStart JSON with the bearer token; never fails Claude), `claude-settings.json` (the SessionStart hook), `opencode-plugin/{package.json,tui.ts}` (route reporter; see conversation-split.md).

## Protocol

Browser ↔ server: the one WebSocket (loopback + Host + Origin checked). `attach` no longer has `create`; `close` is gone. Model requests carry `req` and get `{t:"result", req, ok, id? | error}`:
`addWorkspace{path}`, `createSession{workspaceId, harness}`, `startSession{sessionId}`, `stopSession{sessionId}`, `retryAgent{agentId}`, `restartShell{workspaceId}`.
The server sends `{t:"model", model}` whole on connect and after each change (coalesced per tick). A request's model push goes out before its `result`.

Agent → server (`src/server/agentApi.ts`), POST only:
- `/api/agent/claude-session-start`: hook JSON.
- `/api/agent/opencode-route`: `{seq, route}`.

Checks, else 403:
1. The peer is loopback.
2. `Host` is a loopback name on our port.
3. There is no `Origin` header.
4. `Authorization: Bearer <64 hex>` is a key in the token map (a hash lookup) whose terminal is still that agent's.

Body over 64 KB → 413. Built behaviour: the first SessionStart from a process adopts its `session_id` as the conversation and records `transcriptPath`, and later ones only record the path. OpenCode routes only advance `lastSeq`. Neither switches conversations yet.

## Client

- `modelStore.ts` (`useModel`) is fed by `model` messages; `terminalSocket.ts` has `request()`. The app starts on the overview; "Add workspace" sits centred at the bottom (a path field, inline error); success zooms into the card.
- The session list has "New session" (pick Claude Code / OpenCode) and rows with a state badge and Start / Stop / Retry.
- Panels are keyed by agent id (`TerminalHost` + `getOrCreateView(viewId)` with `setTerminal(id | null)`). A view detaches when no view in the tab shows the terminal any more. With no terminal the panel shows an overlay: stopped → Start, failed → reason + Retry, restarting / stopping. Agent tabs can't be closed.
- Bottom panel: the workspace shell, with "Shell exited (code)" + Restart.

## Tests (built)

`src/server/sessions.test.ts` covers, against the fake harnesses:
- adding a workspace: realpath, `~`, errors, duplicates via a symlink or `/tmp`;
- session naming, launch args and cwd;
- restart and resume, crash loop to failed and retry;
- stop during the backoff, stop killing the session (ps), start while stopping, workers not restarting;
- spawn failure and rollback; shutdown cancelling a pending restart;
- op routing for hot reload, atomic harness files;
- the shell (exit, two restarts at once give one shell);
- the protocol (model before result, reconnect);
- agent API auth (missing, wrong or revoked token; `Origin`; foreign Host; oversize body).

`src/terminalSocket.test.ts`: views never send create/close, follow a replaced terminal, and a terminal moving between views stays attached.
Not covered: the crash-loop count resetting after ≥10s alive.
