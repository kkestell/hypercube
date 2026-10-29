# Conversation switch: /clear, /new, /resume, OpenCode picker (not built)

A Hypercube session is a conversation. When a lead's live process moves to another conversation C, the process goes with C into C's session, and the session it left is stopped.

## Relies on (built in sessions.md)

- **Tokens.** Each agent process gets `HYPERCUBE_TOKEN` (32 random bytes, hex). It is registered before the spawn in `state.tokens` (token → terminal id) and revoked when the process stops being that agent's. `agentForToken(token)` returns the agent whose live terminal holds the token. `Runtime.token` is the current one.
- **Endpoints** (`src/server/agentApi.ts`): `POST /api/agent/claude-session-start` and `/api/agent/opencode-route`. Checks: loopback peer, loopback Host on our port, no `Origin`, a live bearer token; 64 KB cap. They call `claudeSessionStarted` / `opencodeRouteChanged` in `src/server/sessions.ts`, which this plan extends.
- **Adoption.** `claudeSessionStarted` treats the first event from a process as the conversation it opened (`conversationId := session_id`, `Runtime.adopted = true`) and records `transcript_path`. Later events only record the path. That is where the switch goes.
- **Route sequencing.** `opencodeRouteChanged` accepts only `seq > Runtime.lastSeq` (reset per process); stale and duplicate events are no-ops.
- **Transcript check** (`harnessCommand`): resume with `--resume` only if the transcript exists, otherwise `--session-id`. A conversation that has just been switched to has no transcript until something is recorded.
- **Synchronous handlers** (S5): each event looks up the owner of terminal T when it runs, so events that arrive after T moved act on the new owner. Async completions (`closed`, `ended`, `retry`) check `agent.terminalId == T`.
- **Harness files**: `claude-settings.json` already registers the SessionStart hook. `opencode-plugin/` is written and loaded via `OPENCODE_CLI_CONFIG_CONTENT`, but it has **not** been run inside real OpenCode.
- **Views keyed by agent**: a terminal moving from one agent's view to another's stays attached (`unregisterChannel` detaches only when no view shows it).

## Facts (verified on Claude Code 2.1.312, OpenCode 2.2.0)

- Claude `/clear` and `/new` fire SessionEnd(reason=clear, old id), then SessionStart(source=clear, new UUID, transcript_path). Startup fires SessionStart(source=startup) with our `--session-id`. A transcript only exists once something is recorded.
- `claude --help`: `--fork-session` "When resuming, create a new session ID", implying plain `--resume` keeps the id. Adoption is safe either way.
- OpenCode: a TUI plugin can watch `ctx.ui.router.current()`, which is `{type:"session", sessionID}` or `{type:"home"}` after /new. The new conversation id exists only after the first prompt. The server-wide `/api/event` is not per TUI; don't use it.
- Minimal plugin, loaded with no npm install: `package.json {"name":"hypercube-session-tracker","type":"module","exports":{"./tui":"./tui.ts"}}`; `tui.ts` uses `Plugin.define` from `@opencode/plugin/tui` and `createEffect` from `solid-js` inside `ctx.ui.slot({append:"app", render})`.

## To verify first

- The written plugin (`harness.ts` `OPENCODE_PLUGIN`) loads in real OpenCode, `process.env` is visible to it, and it reports a route on startup and after a prompt following /new.
- Whether Claude waits for each SessionStart hook before running the next one (it affects ordering; a duplicate is a no-op either way).

## Detection

For the agent A that currently owns the token's terminal T:
- **claude**: SessionStart after adoption with `session_id != A.conversationId` is a switch to C = `session_id` (/clear, /new, /resume). Other events only record `transcriptPath`.
- **opencode**: route `{type:"session", sessionID}` with a fresh `seq` and `sessionID != A.conversationId` is a switch (first prompt after /new, or picking another conversation). `home` changes nothing. Between /new and the first prompt the model still shows the old conversation, so a stop or restart in that window resumes the previous one. This is intended (user decision).
- Plugin: POSTs `{seq, route}` one at a time through a promise chain, `seq` counting up per process, errors caught.

## Handler (synchronous)

Let X = A's session.
1. Ignore unless A is a lead, A.state == running, A.terminalId == T and X.state == running.
2. Find S ≠ X in the same workspace whose lead L has `conversationId == C`.
   - **S exists:** if L has a process (running / stopping) or a retry timer, unbind it now: revoke its token, clear the timer, close its terminal (its `closed`/`ended` no longer match L). Then `L.terminalId = T`, L.state = running, `Runtime(L)` gets T's token (`adopted = true`, `lastSeq` carried over, `startedAt = now`, `quickExits = 0`), and S.state = running. The old process may live a moment longer while it is killed; it is unbound, so nothing it reports is accepted. This is the only overlap allowed: **a conversation has at most one live, bound process**.
   - **No S:** create S = `Untitled ${nextSessionNumber++}` with a lead L: same harness, `conversationId = C`, `terminalId = T`, runtime as above with `transcriptPath` from the hook.
   - In both cases T's owner becomes L (`state.owners`), and A's runtime token is cleared without revoking it from the map, because it now belongs to L.
3. A: `terminalId = null`, state stopped, `conversationId` unchanged. X: state stopped, and every other X agent is stopped.
4. Push the model and `{t:"switch", fromSessionId: X, toSessionId: S}` (new ServerMessage). Tabs whose active session is X switch to S.

The pid never changes. Starting X later resumes its conversation in a new process.

## Races (handled by the owner check at run time)

- Two switches from T in flight: the second runs after T belongs to L, so it switches S → S′, or does nothing if it names L's conversation.
- `ended(T)` after a switch: it finds L and restarts L in S (`--session-id C` if there is no transcript yet). A `stop(S)` before or after it behaves normally.
- A hook from a replaced or unbound process: its token is revoked → 403.
- A switch while X is stopping: ignored at step 1.
- OpenCode POSTs out of order: ignored by `seq`.

## Tests

With the fake harnesses (`test/fixtures/fake-harness.ts` already supports `/clear` and `route <seq> <id|home>`):
- `/clear` → new session Y with lead B on the same terminal id and pid; X stopped and keeps C1; starting X → `--resume C1` once recorded; the `switch` message is pushed.
- `/resume C1` from Y's process while X is stopped → T moves to X's lead, Y stopped, no new session. The same while X's lead is running → X's old process is killed (ps), T bound to X's lead.
- An OpenCode route to another id → switch; home → nothing; seq 3 before seq 2 → ends on 3; a duplicate seq is ignored.
- A revoked token → 403; first-event adoption creates no session.
- Client: on `switch`, only tabs showing `from` change session.
- Opt-in smoke (`HYPERCUBE_SMOKE=1`) against the real binaries: Claude `/clear` → switch, restart resumes; the OpenCode plugin reports routes.
