# Simplification review, 2026-10-29

Reviewed `7ae6da4` (phases 1–9 and the terminal) with eight read-only reviewers, one lens each: tests, speculative edge cases, server module boundaries, process and environment abstractions, state and persistence, protocol and frontend, the agent runtime, and security plus drift. The brief was less code and fewer tests: findings that add code were accepted only for real bugs. No reviewer found a security bypass.

Findings are grouped by theme, most important first. **Fix** marks what is fixed now; **Defer** goes to `agents/issues.md`; **Rejected** says why.

## 1. Process lifecycle has several owners (Fix)

The biggest theme: who owns a process until it is really gone, and who owns shutdown.

- **Stop drops ownership before the process exits.** `stopAgents` deletes the runtime and then waits for the CLI's exit with a 5 s race; a shutdown during that window neither waits for nor kills the process (reproduced with a fake runtime). Docker's `kill()` is fire-and-forget, so its in-container cleanup can't be awaited at all. (`agents.ts` stop path, `docker.ts` kill, `environment.ts` process type.) Fix: one process contract for piped and PTY processes, `stop(): Promise<void>` (idempotent, graceful, resolves when everything it owns is gone) and `stopNow()` (synchronous, for the exit hook); the runtime stays owned until `stop()` settles, and stop, restart and shutdown all await it.
- **Local `killNow` is SIGTERM.** It is the same function as `kill`, so a process that ignores TERM survives server exit (reproduced). `stopNow()` sends SIGKILL.
- **Deleting a session leaves it `ready` during teardown.** The `deleting` set only blocks terminals; Restart (or Start) during the awaited teardown launches an agent that the delete then orphans (reproduced). Fix: a `deleting` session status replacing the set, so the existing `ready`/`stopped` guards refuse starts.
- **Starting a Docker session marks agents `starting` before their runtimes exist**, so prompts during a slow container start are rejected instead of queued, and Restart skips `ensureRunning`. Fix: `startAgent` registers the runtime, then calls `ensureRunning`, then spawns; delete `launchAgents`.
- **Three modules install process-wide shutdown hooks on import**, each with its own global flag (`sessions.ts`, `agents.ts`, `terminals.ts`). Fix: `index.ts` owns shutdown and calls exported cleanup functions.

## 2. Data safety (Fix)

- **Safe delete passes when the clone can't be reached.** `unsaved.ts` turns any `resolveDir` failure (stopped container, Docker down, permissions) into "nothing to lose", so delete can destroy unchecked work (reproduced). Fix: let it fall into the existing "could not check" path, which asks.

## 3. Agent runtime bugs (Fix)

- **A permission request racing a cancel or steer reopens `waiting`** and holds the steer until someone answers. Fix: answer `cancelled` at once while the turn is being cancelled or a steer is pending.
- **Panel info goes stale after Stop/Start or Restart.** The server resets plan, usage and commands to `{}` without telling the browser, which keeps merging into the old values. Fix: info messages are whole snapshots the browser replaces; a new runtime publishes `{}`.
- The turn logic is otherwise coherent: no lost or double-sent message found. The smallest model is `starting → idle ↔ active → exited`, a FIFO queue, and an active turn owning cancel-requested and a pending steer. No rewrite.

## 4. State and model (Fix, except one)

- **`Session.terminal` mirrors the terminal registry**, kept in sync on create, close and natural exit through a second owner hook. Fix: derive it when the model is sent; delete the hook and the stripping in the store.
- **Membership is stored both ways** (`Workspace.sessionIds` and `Session.workspaceId`; `Session.agentIds` and `Agent.sessionId`). Fix: keep the parent ids only and select children by parent id; insertion order is kept by the store's ordered load.
- **Unused fields:** `Model.version` (incremented, never read), `Session.createdAt`, `Workspace.name` (always the repo), `EnvironmentProcess.pid`, `modelStore`'s `received`/`useModelReceived`. Delete.
- **Transcript rows have three deleters** (`dropItems`, model sync, startup orphan cleanup). Fix: delete orphan items inside the model write's transaction; drop the other two.
- Defer: storing workspaces, sessions and agents as one snapshot row instead of three synced tables. Saves ~15 lines but changes the write pattern; not worth the churn now.

## 5. Environment interface (Fix)

- **Clone-boundary checking is implemented per backend** (`resolveDir` in local and Docker). Fix: one shared helper over `env.run`; drop the interface method and `requireInside`.
- **Agent startup imports Docker to rewrite the MCP URL.** Fix: `Environment.hostUrl(url)` (identity locally).

## 6. Frontend (Fix)

- **HMR can leave two sockets open** (invariant 2): re-evaluating `src/socket.ts` loses the old socket and reconnect timer without closing them (reproduced with a fake WebSocket, not in a browser). Fix: a hot-dispose path that closes them.
- **The terminal attaches at a stale size**: `sized` means "was ever fitted", so after collapsing the panel a session switch or reconnect resizes the shell to old dimensions. Fix: one fit-then-attach step that checks the current size; delete `sized`.
- **`AgentPanel` keeps optimistic copies of server state**: a rejected send restores the old prompt over a newer draft, and an earlier config request's `finally` clears a later choice. Fix: render `option.currentValue`; clear the draft only if unchanged.
- **The client socket supports several views per terminal**; the app has one per workspace. Fix: a plain map.
- Lead-only tools are checked twice in `mcp.ts` (`LEAD_ONLY_TOOLS` and `requireLead`). Fix: one check.

## 7. Tests (Fix)

- **The 200,000-line snapshot test can't detect a lost snapshot**: it checks only the retained tail (shown by dropping the snapshot entirely and still passing). Replace with a few controlled writes at the attach boundary.
- The resize/redraw race test asserts before the restore it is about; merge it with the SIGWINCH test and check the final size.
- The browser socket tests are one order-dependent lifecycle split into four tests that fail alone; merge them.
- Overlapping server socket model tests; eight repeated quick-exit runs duplicating the forced-interleaving test; a test that passes on the shell echoing its own input. Merge or delete.
- **Process-counting helpers match host-wide by fixed names** (`pgrep -f`, global `pkill`), so concurrent runs collide and can kill each other's fixtures. Use per-run markers.
- Keep: the upgrade rejection matrix, the one-socket lifecycle, model-before-result, validation, controlled ordering, and the ownership tests.

## 8. Docs drift (Fix)

- `AGENTS.md` invariant 1 and the architecture notes predate agents: `addWorkspace` no longer takes a path, the panels aren't placeholders, and the blanket Origin rule doesn't fit the bearer-token `/mcp`. The real boundaries are the `[::1]` app listener with the Host and Origin check, and the separate `127.0.0.1` listener serving only `/mcp` with per-agent tokens.
- `docs/architecture.md`'s message tables drift from `src/protocol.ts` (binary terminal frames, `info`, `setConfigOption`, `createSession`'s environment) and wrongly says plan, usage and command updates are dropped. Replace the tables with a pointer to `protocol.ts` and keep the behavior notes.
- `docs/agents.md` and the plan's phase 9 still describe messages waiting for the turn to end, credentials passed as environment variables, and open Docker questions that are now settled.

## Rejected

- **Treat a steer that arrives while an agent is starting as an ordinary queued prompt** (edge-case and agent reviewers). That special case is the fix for a bug the user hit: a lead's message to a worker that was still starting waited behind the worker's whole first task.

## Deferred

- One snapshot row for the structural model (section 4).
- `attachTerminal`'s optional size, used only by tests (low value).
