# Hypercube

A web UI for running swarms of coding agents.

- A **workspace** is a project, e.g. a repo the user works on.
- A **session** is roughly a task inside a workspace. Some are short-lived; some are kept for a long time. A session has an orchestrator the user talks to and workers the orchestrator starts.
- An **agent** is one coding-agent harness (Claude Code or OpenCode) running in a PTY. Agents in a session message each other.

Expected scale: about six workspaces, each with one or two active sessions and many old ones, each session with one to eight agents. Plan for 60–100 live agent processes.

## Architecture

- **The server owns processes.** It keeps a registry of terminals (`src/server/terminals.ts`), stored on `globalThis` so `bun --hot` reloads keep them. Browsers attach and detach; a terminal is spawned only by an explicit create and killed only by an explicit close or server shutdown. This registry is the boundary a background daemon will take over later, so keep the browser talking to it only through the protocol.
- **One WebSocket per browser tab** (`src/server/socket.ts`, `src/terminalSocket.ts`, shared types in `src/protocol.ts`). Each terminal is a pub/sub topic; a client subscribes only to the terminals it shows. A separate low-rate status stream reports every terminal's state without output bytes.
- **Snapshots.** The server keeps a headless xterm per terminal, so a client that attaches gets the current screen and scrollback, then live output.
- **Background workspaces cost nothing in the browser** beyond the status stream: no output delivery, no xterm parsing for terminals the user can't see.
- **Stopped sessions hold no processes** (planned): they keep scrollback and the harness's conversation id and resume on reopen.

## Invariants

These must hold. A change that touches one needs a test that forces the interleaving or edge case that would break it, not just the happy path.

1. **Security.** The server runs shells. Bind to loopback only. Accept a WebSocket upgrade only when `Host` is a loopback name and `Origin` exactly equals the server's own origin (scheme, host, port). Any new endpoint that spawns a process or writes to one gets the same check.
2. **Ownership.** A terminal owns every process in its PTY's session, including background, `nohup`ed and reparented ones. A process that detaches into its own session (`setsid`, daemons) is no longer the terminal's and is left running, as a terminal emulator would. Closing the terminal kills everything it owns. Server shutdown kills all of them, including terminals that are mid-close. Never signal a PID unless you know it is still ours; PIDs get reused.
3. **Lifecycle is explicit.** A socket disconnect, page reload, HMR update or server hot reload never spawns or kills a process.
4. **Per-terminal ordering.** A client sees the snapshot, then output in order, then exit. Exit never arrives before output written before it. The snapshot-to-live handoff has no gap and no duplicate bytes.
5. **Snapshots are best effort.** The serializer can't capture all terminal state (partial escape sequences, character sets, saved cursor, open hyperlinks), so after a client attaches the server makes the app redraw. A restored screen only has to be right after that redraw. Don't add code to make snapshots exact.
6. **Resize ordering.** Output is parsed at the size it was written at; a resize applies after all earlier output, and a snapshot reflects the size the client asked for.
7. **One socket.** A tab has at most one open or connecting socket. Events from a stale socket are ignored.

## Reviewing

Report findings that can kill the wrong process, leak a process, spawn one nobody asked for, expose the shell endpoint, or lose, duplicate or reorder output. Don't report cosmetic differences in a restored screen that the post-attach redraw fixes.

## Working here

- The user's dev server (`bun dev`, port 3000) runs with `--hot` from this checkout, so your edits go live in it. Never stop it. Run your own instance on another port: `PORT=3101 bun --hot src/index.ts`.
- Before reporting work as done, run `bun test` and `bunx tsc --noEmit`, and say what you ran and what it showed.
- Test the behavior a user would notice: processes cleaned up, nothing lost across reloads, lifecycle rules, security checks. Don't add a test per edge case or per review finding; a precisely timed race gets a test only if hitting it kills the wrong process, leaks one, or exposes the shell endpoint. Keep the suite fast.
- Tests that start processes must clean them up. For process-cleanup tests, check with `ps` that nothing survives.
- Don't claim something is race-free, leak-free or secure unless a test would fail if it weren't. State what you verified and what you didn't.
- Don't commit or create branches unless asked.

## Bun

Use Bun, not Node tooling: `bun <file>`, `bun test` (`bun:test`), `bun install`, `bun run`, `bunx`. Bun loads `.env` itself.

- `Bun.serve()` for HTTP, routes and WebSockets; no Express or `ws`.
- HTML imports with `Bun.serve()` for the frontend; no Vite. HTML can import `.tsx` and CSS directly, and Tailwind is wired up through `bun-plugin-tailwind`.
- `bun:sqlite` for SQLite, `Bun.file` over `node:fs` read/write, `` Bun.$`cmd` `` over execa.
- Bun API docs are in `node_modules/bun-types/docs/**.mdx`.
