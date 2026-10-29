# Hypercube

A web UI for running swarms of coding agents.

- A **workspace** is a project, e.g. a repo the user works on.
- A **session** is roughly a task inside a workspace. Some are short-lived; some are kept for a long time. A session has an orchestrator the user talks to and workers the orchestrator starts.
- An **agent** is one coding-agent harness (Claude Code or OpenCode). Agents in a session message each other.

## Current state

`docs/agents.md` is the design: a workspace is a GitHub repo, a session is a clone of it, and agents are OpenCode processes Hypercube drives over ACP and renders itself. It is being built in phases from `agents/plans/agents.md`; `docs/architecture.md` describes what exists now. Problems found along the way and left for later go in `agents/issues.md`. The earlier terminal-based version is on branch `process-management`.

OpenCode is logged in with the user's ChatGPT subscription. Agents run on `openai/*` models; OpenCode's own default model is a paid OpenRouter one, so Hypercube always sets the model.

Expected scale, once agents run: about six workspaces, each with one or two active sessions and many old ones, each session with one to eight agents.

## Architecture

`docs/architecture.md` describes the routes, the WebSocket protocol and the model in detail. Keep it current when you change them.

- **The server owns the model.** Workspaces, sessions and agents live in memory in `src/server/sessions.ts`, on `globalThis` so `bun --hot` reloads keep them. They are also stored in SQLite (`src/server/store.ts`, `~/.local/share/hypercube/hypercube.db`, or `$HYPERCUBE_DB`), and agents are resumed at startup. The browser only asks for changes and is pushed the whole model.
- **One WebSocket per browser tab** (`src/server/socket.ts`, `src/socket.ts`, shared types in `src/protocol.ts`). The server sends the model on connect and after every change; the browser sends `addWorkspace` and `createSession` requests and gets a `result` for each.
- **Frontend.** `App` lays out the overview and the workspace cards; `WorkspaceView` has the session list, the session's dockview layout (`SessionView`) and the placeholder panels.

## Invariants

These must hold. A change that touches one needs a test that forces the interleaving or edge case that would break it, not just the happy path.

1. **Security.** Bind to loopback only. Accept a WebSocket upgrade only when `Host` is a loopback name and `Origin` exactly equals the server's own origin (scheme, host, port). Any new endpoint that changes state gets the same check. `addWorkspace` takes a path from the browser, so it is only as safe as that check.
2. **One socket.** A tab has at most one open or connecting socket. Events from a stale socket are ignored.

## Terminals

Each session can have a shell terminal in the bottom panel (`src/server/terminals.ts`, `src/server/localPty.ts`), ported from branch `process-management`. These rules hold for it and for anything else that runs processes:

- **Ownership.** A terminal owns every process in its PTY's session, including background, `nohup`ed and reparented ones. A process that detaches into its own session (`setsid`, daemons) is no longer the terminal's and is left running. Closing the terminal kills everything it owns. Server shutdown kills all of them, including terminals that are mid-close. Never signal a PID unless you know it is still ours; PIDs get reused.
- **Lifecycle is explicit.** A socket disconnect, page reload, HMR update or server hot reload never spawns or kills a process. A terminal is spawned only by an explicit create and killed only by an explicit close (closing it, or stopping or deleting its session) or server shutdown.
- **Per-terminal ordering.** A client sees the snapshot, then output in order, then exit. Exit never arrives before output written before it. The snapshot-to-live handoff has no gap and no duplicate bytes.
- **Snapshots are best effort.** The serializer can't capture all terminal state, so after a client attaches the server makes the app redraw. Don't add code to make snapshots exact.
- **Resize ordering.** Output is parsed at the size it was written at; a resize applies after all earlier output, and a snapshot reflects the size the client asked for.

Review findings then are those that can kill the wrong process, leak a process, spawn one nobody asked for, expose the shell endpoint, or lose, duplicate or reorder output.

## Working here

- The user's dev server (`bun dev`, port 3000) runs with `--hot` from this checkout, so your edits go live in it. Never stop or restart it unless the user asks. Don't leave the tree broken for long. Run your own instance on another port: `PORT=3101 bun --hot src/index.ts` (its MCP listener takes the next port, 3102), and stop it when you're done.
- Before reporting work as done, run `bun test` and `bunx tsc --noEmit`, and say what you ran and what it showed.
- Test the behavior a user would notice: model changes, validation, the security check. Don't add a test per edge case or per review finding. Keep the suite fast.
- Don't claim something is race-free or secure unless a test would fail if it weren't. State what you verified and what you didn't.
- Don't commit or create branches unless asked, and don't offer to.

## Building in phases

While the phases in `agents/plans/agents.md` are being built, speed matters more than polish, and the user tests the UI themselves on port 3000.

- **Verify cheaply.** Run `bun test` and `bunx tsc --noEmit`, plus the smallest server-side check that proves the phase works: `curl`, or one short WebSocket script against your own port. No browser automation of any kind, and don't install browser tooling. Say which parts of the UI you didn't see rendered.
- **Real agents cost money.** Keep test prompts tiny, clone only small public repos, and delete the sessions you create.
- **Stay in scope.** Build only the phase you were given. Anything else you notice goes in your report, not in the code.
- **Model shape changes.** The model and transcripts live in memory across `bun --hot` reloads, so records made by older code keep their old shape. Say in your report when a change needs a fresh server or new sessions to try.

## Orchestrating workers

When Hypercube workers do the phases:

- **Profiles.** Sonnet (`claude-medium`) implements; Opus (`claude-large`) reviews. Don't hand reviews to cheaper models.
- **Briefs.** Tell the worker everything it would otherwise have to discover: files to read, how to verify (the rules above), what not to touch, when to stop. Ask for a short report, not a transcript.
- **Hand over at once.** When the implementer reports done, tell the user what to test in the same message and start the review in parallel. Don't make the user wait for review.
- **Quick reviews.** The reviewer reads code only and looks for departures from the plan, correctness bugs a user would hit, security holes, scope creep and over-engineering. Fix what would stop the user testing; write the rest to `agents/issues.md`.
- **Tell the user only what they can act on:** what's ready, what to test, what's blocked.

## Code style

- Do less. Keep correct code simple, ordinary, and cheap to change.
- Write idiomatic TypeScript and React. Prefer plain data and direct functions.
- Give each concern a clear boundary and each lifecycle one owner.
- Let real needs earn abstractions, dependencies, and configuration. Measure
  before optimizing; harden against observed failures.
- Return clear errors for bad input; fail loudly on broken invariants. Test
  observable behavior; write comments that explain why.

## Bun

Use Bun, not Node tooling: `bun <file>`, `bun test` (`bun:test`), `bun install`, `bun run`, `bunx`. Bun loads `.env` itself.

- `Bun.serve()` for HTTP, routes and WebSockets; no Express or `ws`.
- HTML imports with `Bun.serve()` for the frontend; no Vite. HTML can import `.tsx` and CSS directly, and Tailwind is wired up through `bun-plugin-tailwind`.
- `bun:sqlite` for SQLite, `Bun.file` over `node:fs` read/write, `` Bun.$`cmd` `` over execa.
- Bun API docs are in `node_modules/bun-types/docs/**.mdx`.
