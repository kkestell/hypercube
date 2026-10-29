# Issues

Found while building the phases in `agents/plans/agents.md`, left for later.

## Phase 1

- No way to delete a workspace. Deleting one should delete its sessions (and their clones) first.
- A session stays `ready` in the model while its teardown runs, so another tab can send a second delete. Harmless today (forced rm, idempotent removal).

## Phase 2

- `src/server/environment.ts` `kill()` returns early once the opencode leader has exited, so its process group is never signalled: children it started (e.g. a dev server) outlive a crashed lead and survive session delete. Always signal `-pid` and ignore ESRCH (phase 10 ownership work).
- Permission prompts are only exercised when OpenCode is configured to "ask"; the user's default config never asks.
- `bun dev` on port 3000 stopped once during phase 2 work with nothing in its log saying why.

## Phase 3

- Session title: only the first `session_info_update` title is applied (the name must still be "Untitled N"). If OpenCode sends a placeholder title first, the session keeps it. Unverified.
- Usage with a context size of 0 shows "NaN%" (`src/AgentPanel.tsx`).
- Transcript items live in memory across `bun --hot` reloads, so a change to an item's shape crashes the panel on transcripts made by older code (happened with tool items going from phase 2 to 3). Restarting the server clears it; persistence (phase 6) will need a real answer.

## Phase 4

- `agentForToken` still accepts an exited agent's token (the runtime is kept until the session is deleted), so its tools keep working. Matters only if a child outlives opencode.
- The preamble is cleared before the first prompt is sent; if that turn fails, the lead never gets it.
- Every lead is named "Lead", so `send_message` by name is ambiguous with two or more leads; leads have to list agents and use ids.

## Phase 5

- The lead can't see a worker's reply until its own turn ends: messages queue behind the turn. A lead that waits for a worker by polling inside its turn (a sleep loop in one tool call) never receives the message until it gives up. Seen once in a real run. The lead preamble now says to end the turn instead of polling; unverified whether leads follow it.
- A bad profile's error lists the first 20 of the values OpenCode offers, alphabetically. OpenCode offers hundreds of models (every provider), so the useful ones may not show.
- `~/.config/hypercube/config.json` is the Rust Hypercube's file (it has `orchestrator`, Claude profiles and OpenRouter profiles). Hypercube skips profiles whose harness isn't `opencode` and uses the built-in lead when there is no `lead`, so the file works as is; the OpenRouter profiles it offers cost money. Whether `lead` should be required, or the file shared at all, is the user's call.
- Removing a worker leaves its transcript in the browser's `transcriptStore` until the page reloads (the server dropped it).
- Starting a worker is refused only for a name clash and a bad directory. No limit on workers, and no check that two workers aren't given the same directory.
- An exited worker stays in the session until the lead or the user removes it; nothing tells the lead except `list_agents`.
- `start_worker` replies "Started worker" before the profile is applied, so a bad profile isn't reported to the lead; it only shows in `list_agents`. The `exited` check after start in `src/server/mcp.ts` never fires.

## Phase 6

- Every agent of every ready session starts at once on server startup, one `opencode acp` each. Fine for a few sessions; stagger it if many sessions are left running.
- The prompt queue isn't stored: prompts waiting at a stop, restart or server restart are lost, including messages other leads were told were "Sent to …".
- All transcripts are loaded into memory at startup and all sent to every new socket. Old sessions make that grow.
- Panel info (plan, usage, commands) isn't stored; a stopped or restarted agent's header is empty until it sends them again.
- A change to the stored shape (an item, an agent) needs a new `SCHEMA_VERSION`, which drops the whole database and leaves the clones on disk.
- The safe-delete check runs before the agents are stopped, so something an agent writes between the check and the kill is lost unnoticed.
- Start while a Stop is still waiting for the processes (up to 5 s) launches the new process beside the dying one. The old one's events are ignored.
- Restarting a crashed agent doesn't clean up what its old process group left: `kill()` in `src/server/environment.ts` returns early once the leader has exited (see Phase 2).
- A repo with no commits makes `git log HEAD` fail, so deleting such a session always asks, with "Could not check".
- The delete confirmation is a native `confirm()` with the list as text, capped at 8 lines per section.
- `session/load` (the fallback after `session/resume`) was checked once, with resume forced off in a copy of the source. OpenCode advertises resume, so it never runs in practice.
- Review: Restart, then Stop, then Start while `restartAgent` is still awaiting `stopAgents` makes both call `launch()`: two opencode processes on one ACP session, the first runtime leaked. Tiny window. Fix: `startAgent` stops any existing runtime, or re-check `status === "starting"` before launch (`src/server/sessions.ts` `restartAgent`).
- Review: a worktree nested more than one level deep (e.g. `.worktrees/foo`) shows in the clone's status as `?? .worktrees/`, which the safe-delete filter doesn't drop, so the user is asked about untracked files that aren't there (`src/server/unsaved.ts`). Extra confirm only.
- Review: `saveItem` rewrites the whole item on every streamed chunk. Fine at this scale.

## Phase 7

- A tool call that hasn't started (`pending`, e.g. waiting on a permission) isn't waited for: a steer cancels it at once. Only `in_progress` calls are.
- A tool that hangs holds up every steer, including agents' messages, until the user presses Interrupt now in that agent's panel; nothing interrupts it automatically.
- Queued prompts that were in memory across the hot reload that added `sentAt` have none, and would say "NaN minutes ago" when sent.
- Composer controls are functional only: Enter steers, ⌘/Ctrl+Enter or the Queue button queues, Steer replaces Send while the agent works.
- Review: several steers arriving before the turn stops are merged into one prompt. The plan listed that as deferred; it's small and stays.
- There is no way to remove a single queued prompt; Stop now keeps the queue (it used to drop it, losing messages from other agents).


## Phase 8

- The plan's "recent messages between agents" side-panel content isn't built; the bottom panel is still empty.
- `src/WorkspaceView.tsx` `docks` keeps the dockview APIs of deleted sessions until the workspace unmounts. Unused, harmless.
- Overview button gets an amber dot when any agent anywhere waits on the user: added by the worker, not in the plan.


## Terminal

- `startTerminal` and `startSession` aren't refused once shutdown has started (the branch's `refuseDuringShutdown` was dropped). A shell started in the ~2 s grace is SIGKILLed by the exit handler without a grace period. Nothing leaks.
- When the shell exits by itself, the panel resets to the Start button at once, so its last output (why it exited) is lost. With background jobs left, the dead terminal shows no exit indication for up to the ~2 s reap.
- Signal and exit handlers are installed once per process behind `globalThis` guards, so a hot-reloaded server keeps older handlers until it is restarted.


## Phase 9

- Review: a terminal's reap runs the container `pkill -s <pid>` after the shell has exited, so a pid the container has since reused could be signalled (`src/server/docker.ts` STOP). Low risk.
- `opencode auth import` leaves a background `opencode serve --service` in the container; it goes with the container.
- The server log shows a harmless "setsid: child N did not exit normally" when a Docker agent is killed.
- Docker workers on a profile other than `openai/*` fail: containers get only the OpenAI login.
- A command a Docker agent's OpenCode runs in the background (seen: a lead's `nohup sleep 301 &`) is in a session of OpenCode's own, so stopping the agent or shutting the server down leaves it running in the container; only deleting the session ends it. Terminals are not affected. Same gap as the phase 2 note on children of a lead.
- `restartAgent` doesn't `ensureRunning`: after Docker restarts under a running server, an agent's Restart fails; Stop then Start on the session works.
- While Start or startup waits for `docker start`, the session is `ready` and a terminal started in that moment finds a stopped container and exits at once.

## UI (fix once the core is built)

User feedback after phase 4:

1. The tab's green dot and status text ("idle") look absurd. Nobody can tell what the dot means, and status text doesn't belong in a tab title.
2. The composer and Send button look wrong. Chat composers are a solved UX problem; follow the usual pattern.
3. The model, effort and mode selects sit above the transcript. Rethink where they go.
4. The transcript looks generally awful and needs a proper design pass.
