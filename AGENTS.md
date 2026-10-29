# Hypercube

A web UI for running swarms of coding agents.

- A **workspace** is a project, e.g. a repo the user works on.
- A **session** is roughly a task inside a workspace. Some are short-lived; some are kept for a long time. A session has an orchestrator the user talks to and workers the orchestrator starts.
- An **agent** is one coding-agent harness (Claude Code or OpenCode). Agents in a session message each other.

## Current state

The app is the workspace and session UI only. It runs no processes: there are no terminals, harnesses or agent API. Each session has one agent record, the lead (role `lead`, name "Lead"), shown as an empty dockview panel. The full version, with process management, is on branch `process-management`.

Expected scale, once agents run: about six workspaces, each with one or two active sessions and many old ones, each session with one to eight agents.

## Architecture

`docs/architecture.md` describes the routes, the WebSocket protocol and the model in detail. Keep it current when you change them.

- **The server owns the model.** Workspaces, sessions and agents live in memory in `src/server/sessions.ts`, on `globalThis` so `bun --hot` reloads keep them. Nothing is persisted yet: restarting the server forgets them. The browser only asks for changes and is pushed the whole model.
- **One WebSocket per browser tab** (`src/server/socket.ts`, `src/socket.ts`, shared types in `src/protocol.ts`). The server sends the model on connect and after every change; the browser sends `addWorkspace` and `createSession` requests and gets a `result` for each.
- **Frontend.** `App` lays out the overview and the workspace cards; `WorkspaceView` has the session list, the session's dockview layout (`SessionView`) and the placeholder panels.

## Invariants

These must hold. A change that touches one needs a test that forces the interleaving or edge case that would break it, not just the happy path.

1. **Security.** Bind to loopback only. Accept a WebSocket upgrade only when `Host` is a loopback name and `Origin` exactly equals the server's own origin (scheme, host, port). Any new endpoint that changes state gets the same check. `addWorkspace` takes a path from the browser, so it is only as safe as that check.
2. **One socket.** A tab has at most one open or connecting socket. Events from a stale socket are ignored.

## When terminals return

These apply once the app runs processes again. They were written for, and are implemented and tested on, branch `process-management`; read that branch's `AGENTS.md` and `src/server/terminals.ts` before bringing terminals back.

- **Ownership.** A terminal owns every process in its PTY's session, including background, `nohup`ed and reparented ones. A process that detaches into its own session (`setsid`, daemons) is no longer the terminal's and is left running. Closing the terminal kills everything it owns. Server shutdown kills all of them, including terminals that are mid-close. Never signal a PID unless you know it is still ours; PIDs get reused.
- **Lifecycle is explicit.** A socket disconnect, page reload, HMR update or server hot reload never spawns or kills a process. A terminal is spawned only by an explicit create and killed only by an explicit close or server shutdown.
- **Per-terminal ordering.** A client sees the snapshot, then output in order, then exit. Exit never arrives before output written before it. The snapshot-to-live handoff has no gap and no duplicate bytes.
- **Snapshots are best effort.** The serializer can't capture all terminal state, so after a client attaches the server makes the app redraw. Don't add code to make snapshots exact.
- **Resize ordering.** Output is parsed at the size it was written at; a resize applies after all earlier output, and a snapshot reflects the size the client asked for.

Review findings then are those that can kill the wrong process, leak a process, spawn one nobody asked for, expose the shell endpoint, or lose, duplicate or reorder output.

## Working here

- The user's dev server (`bun dev`, port 3000) runs with `--hot` from this checkout, so your edits go live in it. Never stop it. Run your own instance on another port: `PORT=3101 bun --hot src/index.ts`.
- Before reporting work as done, run `bun test` and `bunx tsc --noEmit`, and say what you ran and what it showed.
- Test the behavior a user would notice: model changes, validation, the security check. Don't add a test per edge case or per review finding. Keep the suite fast.
- Don't claim something is race-free or secure unless a test would fail if it weren't. State what you verified and what you didn't.
- Don't commit or create branches unless asked.

## Bun

Use Bun, not Node tooling: `bun <file>`, `bun test` (`bun:test`), `bun install`, `bun run`, `bunx`. Bun loads `.env` itself.

- `Bun.serve()` for HTTP, routes and WebSockets; no Express or `ws`.
- HTML imports with `Bun.serve()` for the frontend; no Vite. HTML can import `.tsx` and CSS directly, and Tailwind is wired up through `bun-plugin-tailwind`.
- `bun:sqlite` for SQLite, `Bun.file` over `node:fs` read/write, `` Bun.$`cmd` `` over execa.
- Bun API docs are in `node_modules/bun-types/docs/**.mdx`.
