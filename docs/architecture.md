# Architecture

Hypercube is a Bun server and a React app served by it. The server owns the state; the browser displays it and asks for changes over one WebSocket.

```
Browser tab                                   Bun server (src/index.ts)
───────────                                   ─────────────────────────
React UI ── useModel() ── modelStore          GET /*   → index.html (bundled React app, HMR in dev)
                              ▲               GET /ws  → WebSocket upgrade (loopback + Origin check)
                              │ model
src/socket.ts ◀═══════ one WebSocket ═══════▶ src/server/socket.ts ── src/server/sessions.ts (model)
     request() ── addWorkspace / createSession / deleteSession / stopSession / startSession / restartAgent / removeWorker / startTerminal / closeTerminal / prompt / interrupt / cancel / answerPermission ──▶
     attach / detach / input / resize ◀──▶ terminal bytes (binary frames) ── src/server/terminals.ts ── a PTY from the session's Environment
                                              src/server/agents.ts ── `opencode acp` per agent (ACP over stdio)
                                                      ▲   src/server/store.ts ── SQLite (model and transcripts)
                                              POST /mcp ── src/server/mcp.ts (agents call Hypercube's tools; token per agent;
                                                           a second listener, on 127.0.0.1; the app above is on [::1])
```

## Files

| File | Role |
|---|---|
| `src/index.ts` | Two `Bun.serve` listeners. The app (the page and `/ws`) is on `[::1]`. `/mcp`, and nothing else, is on `127.0.0.1`, on the app's port plus one (or `$HYPERCUBE_MCP_PORT`). See "Listeners". It also owns shutdown (see "Lifetime"). |
| `src/model.ts` | Model types, shared by server and browser. |
| `src/protocol.ts` | WebSocket message types, shared by server and browser. |
| `src/server/sessions.ts` | The model and its mutations (`addWorkspace`, `createSession`, `deleteSession`, `stopSession`, `startSession`, `restartAgent`, `startWorker`, `removeWorker`). Starts the lead when a session's clone is ready, stops a session's agents, workers included, before deleting it, and at startup loads the database and resumes the agents (`restoreModel`). `getPushedModel` is the model as browsers get it. |
| `src/server/store.ts` | The SQLite database: loads and saves the model and the transcript items. |
| `src/server/unsaved.ts` | What deleting a session would lose: uncommitted changes and commits on no remote branch, in the clone and its worktrees. |
| `src/server/config.ts` | Reads and validates `~/.config/hypercube/config.json`: the lead's settings and the profiles. |
| `src/server/agents.ts` | Agent processes: spawns `opencode acp`, speaks ACP to it, keeps the prompt queue, status and transcript. Applies the lead's or a profile's model and effort. Holds each agent's MCP token and its preamble. |
| `src/server/mcp.ts` | `POST /mcp` on the MCP listener: Hypercube's MCP server for agents (`list_agents`, `send_message`, `list_profiles`, `start_worker`, `stop_worker`) and who may call each. |
| `src/server/errors.ts` | `ModelError`: a request failure the user can fix. `UnsavedWorkError`: a delete refused for unsaved work. |
| `src/server/environment.ts` | The `Environment` seam (`setup`, `ensureRunning`, `spawn`, `spawnPty`, `loginShell`, `run`, `teardown`, `hostUrl`), the `Stoppable` contract every process has, `resolveDir` (the clone-boundary check, over `run`, for any environment), its local implementation, and `environmentFor(session)`. Owns session clones and command execution. |
| `src/server/docker.ts` | The Docker implementation of `Environment`: one container per session, built from `docker/Dockerfile`; `docker exec` for processes, with in-container process cleanup. |
| `src/server/localPty.ts` | The local `spawnPty`: a PTY, and ownership of every process in its session (how they are found and stopped). `PtyProcess` is what the terminal registry needs from any environment. |
| `src/server/terminals.ts` | The terminal registry: one headless xterm per PTY for snapshots, output batching and ordering, resize ordering, forced redraw on attach, closing. Knows nothing about sessions. |
| `src/server/socket.ts` | Upgrade check, pushes the model and transcript items, dispatches requests, and carries terminal input and output. |
| `src/socket.ts` | The tab's socket: connects, reconnects, `request()` (rejects with `RequestError`, which carries `unsaved` for a refused delete), and the terminal channels (`registerChannel`, `sendTerminal`). |
| `src/terminals.ts` | The browser's xterm view of one session's terminal; lives outside React, keyed by workspace. |
| `src/modelStore.ts` | The last model received; `useModel()` for components. |
| `src/transcriptStore.ts` | Each agent's transcript as last received; `useTranscript(agentId)`. |
| `App.tsx`, `WorkspaceView.tsx`, `SessionList.tsx`, `SessionView.tsx`, `AgentPanel.tsx`, `AgentsPanel.tsx`, `TerminalPanel.tsx` | UI. |
| `src/status.ts` | `activity()`: which agents are busy and which are waiting on the user. The overview cards, session rows, the right panel and the tabs' marker all derive from the pushed model with it; nothing is stored or sent for status. |

## HTTP routes

| Route | Purpose |
|---|---|
| `GET /ws` | WebSocket upgrade; it also carries the terminals' input and output, so it is the only door to a shell. Returns 403 unless the request is from a loopback address, `Host` is `localhost`, `127.0.0.1` or `[::1]` on the server's port, and `Origin` is exactly `http://<Host>`. |
| `POST /mcp` | On the MCP listener only. Hypercube's MCP server for agents, MCP over streamable HTTP. See "MCP server" below. 403 unless the request is from a loopback address with a loopback `Host` (or `host.docker.internal`, which agents in containers use) on the listener's port, and carries no `Origin` header; 401 unless `Authorization: Bearer <token>` is an agent's token; 405 for other methods. |
| `GET /*` | The React app (`index.html`). |

There are no other HTTP endpoints. The app's own traffic goes over the WebSocket.

### Listeners

Docker Desktop makes a process in a container that connects to `host.docker.internal` arrive on the host's `127.0.0.1`, from a loopback peer, so it passes the loopback checks that guard `/ws` and could start a shell on the host. So the listeners are split:

- **The app** (`/`, `/ws`) listens on `[::1]`, which Docker's proxy doesn't reach (it connects over IPv4): a container is refused. `http://localhost:<port>` still works in a browser, which tries `::1` too. Port `$PORT`, else 3000.
- **MCP** (`/mcp` only; every other path is 404, and there is no WebSocket) listens on `127.0.0.1`, port `$HYPERCUBE_MCP_PORT`, else the app's port plus one. Local agents and agents in containers (as `host.docker.internal`) both use it. Whatever is on this listener is open to every container, so it only serves what needs an agent's token.

Verified from a container: `/mcp` on the MCP port answers (401 without a token, and agents' calls work with theirs); the app's port is refused.

## WebSocket protocol

Messages are JSON text frames, except terminal output, which is binary. `src/protocol.ts` has the types; the tables below say what each one does.

**Server → browser**

| Message | When |
|---|---|
| `{ t: "model", model }` | On connect, and after every change. Always the whole model; the browser replaces its copy. |
| `{ t: "transcript", agentId, items }` | On connect, once per agent that has a transcript. The browser replaces its copy. |
| `{ t: "item", agentId, item }` | Whenever an agent's transcript gains an item or an item changes, to every socket. The browser replaces the item with the same `id`, or appends it. |
| `{ t: "info", agentId, info }` | An agent's whole panel info (plan, usage, commands, config options): on connect, after every change, and as `{}` when a new process starts for the agent. The browser replaces its copy. |
| binary frame `[kind u8][idLength u8][id utf8][payload]` | Terminal bytes for a terminal this socket has attached to; `id` is the session's id. Kind 1 is a snapshot (the screen and scrollback, as escape sequences; the browser resets its xterm and writes it), kind 2 is live output. |
| `{ t: "result", req, ok: true, id? }` | Reply to a request. `id` is the created workspace or session. |
| `{ t: "result", req, ok: false, error, unsaved? }` | A request failed; `error` is shown to the user. `unsaved` (a list of `UnsavedWork`) is set when a delete was refused because the session holds work that would be lost. |

**Browser → server**

| Message | Effect |
|---|---|
| `{ t: "addWorkspace", repo, req }` | Accepts `owner/name`, `https://github.com/owner/name[.git]` or `git@github.com:owner/name[.git]`, normalizes to `owner/name`, and checks it exists with `gh repo view`. Rejects an exact duplicate normalized repo. |
| `{ t: "createSession", workspaceId, environment, req }` | `environment` is `local` or `docker`. Creates "Untitled N" (N counts up per workspace, never reused) with one agent, the lead, and status `cloning`. Replies immediately; cloning runs in the background and pushes a model with status `ready` or `failed` and an error message. |
| `{ t: "deleteSession", sessionId, confirmed?, req }` | Checks the clone and every worktree in it for uncommitted changes and commits on no remote branch (see "Deleting a session"), sets the session `deleting`, stops its agents and terminal, removes the clone, then removes the session, its agents and their transcripts. Refused while cloning or already deleting. Refused with `unsaved` set when there is unsaved work or the clone can't be checked, unless `confirmed`; the browser asks the user and sends the request again with `confirmed: true`. A failed (clone-less) session is not checked. Replies without an `id`. |
| `{ t: "stopSession", sessionId, req }` | Stops all of a ready session's agents, workers included, and sets the session `stopped`. Their records and transcripts stay. Replies when the processes are gone. |
| `{ t: "startSession", sessionId, req }` | Starts a stopped session's agents again, each resuming its ACP session. Replies at once; the agents come up as `starting`. |
| `{ t: "restartAgent", agentId, req }` | Starts an exited agent's process again, resuming its ACP session. Replies at once; the agent comes up as `starting`. Refused unless the session is ready and the agent exited, so a second restart while the first runs is refused, and so is one during a delete. |
| `{ t: "removeWorker", agentId }` | Stops a worker (its process group) and removes it and its transcript from the session. Refused for a lead or an unknown agent. The remove action on a worker's tab. |
| `{ t: "startTerminal", sessionId, req }` | Starts the session's shell (see "Terminals"). Refused unless the session is `ready`, or if it already has a terminal, or if the shell can't be started (the error is the process's). |
| `{ t: "closeTerminal", sessionId, req }` | Kills the shell and everything it started; replies when they are gone. Nothing happens if the session has none. |
| `{ t: "attach", id, cols, rows }` | No reply. Resizes the terminal to the tab's size, sends a snapshot frame, then subscribes the socket to the terminal's output. Ignored if the session has no terminal. |
| `{ t: "detach", id }` | No reply. Stops that terminal's output to this socket. |
| `{ t: "input", id, data }` | No reply. Keystrokes for the shell. |
| `{ t: "resize", id, cols, rows }` | No reply. Sizes (clamped to 1–1000) apply after all output written before them. |
| `{ t: "prompt", agentId, text, queue? }` | Sent at once if the agent is idle. If it is busy, the prompt steers it (see "Steering"), or with `queue: true` waits in the queue for the turn to end. Refused if the agent has exited. The prompt is the user's own, so it has no `from`. |
| `{ t: "interrupt", agentId }` | Cancels the agent's turn now instead of after its running tool call; the pending steer follows. Does nothing if no steer is pending. Interrupt now in the panel. |
| `{ t: "cancel", agentId }` | Cancels the agent's current turn and answers its open permission requests as cancelled. Queued prompts and a pending steer are kept and go out next. |
| `{ t: "answerPermission", agentId, requestId, optionId }` | Answers an open permission request with one of its options. Refused for a request that is no longer open or an unknown option. |
| `{ t: "setConfigOption", agentId, configId, value }` | Sets one of the agent's ACP config options (model, effort, mode) with `session/set_config_option`; the agent's reply updates its `info`. |

`req` is a number the browser picks per request; the matching `result` carries it back. A change's `model` push is sent before its `result`, so when `request()` resolves the new object is already in the store. Pushes are coalesced: several changes in one tick send one `model`. Unknown or malformed messages are ignored.

The browser's `request()` rejects if the socket isn't open, and all pending requests reject if it drops. The socket reconnects on its own (250 ms backoff doubling to 5 s) and gets the full model again on reconnect. A tab never has more than one socket open or connecting.

## Model

```ts
Model     = { workspaces: Workspace[], sessions: Record<id, Session>, agents: Record<id, Agent> }
Workspace = { id, repo /* owner/name */, nextSessionNumber }
Session   = { id, workspaceId, name, environment: "local" | "docker", clonePath,
              status: "cloning" | "ready" | "failed" | "stopped" | "deleting", error?,
              terminal? /* true while the session's shell runs; set when the model is sent, never stored */ }
Agent     = { id, sessionId, role: "lead" | "worker", name, profile? /* workers */,
              cwd /* the clone for a lead, a directory inside it for a worker */,
              status: "starting" | "idle" | "busy" | "waiting" | "exited",
              error?, acpSessionId?, queue: QueuedPrompt[], steer?: PendingSteer }
QueuedPrompt = { text, from?, fromId?, sentAt }   // from: the sending agent as "name (session)"; absent for the user's own prompts
PendingSteer = { prompts: QueuedPrompt[], waitingFor? /* title of the tool call it waits for */ }
UnsavedWork  = { path /* relative to the clone, "." for the clone */, uncommitted: string[], unpushed: string[], error? }
```

A child names its parent (`workspaceId`, `sessionId`) and nothing lists children. They are kept in insertion order, which is the display order: a session's agents are its lead, then its workers as they were started, and the database loads them in that order. `terminal` is derived from the terminal registry each time the model is sent; whatever ends a terminal pushes the model again.

The model lives in server memory on `globalThis`, so it survives page reloads and `bun --hot` reloads, and is stored in a database so it survives a server restart (see "Persistence").

A `stopped` session is a ready one whose agents' processes are stopped: its agents are `exited` with no `error`, and Start resumes them. Stopped sessions don't start on server startup. A `deleting` session is being torn down: the guards that need `ready` or `stopped` (Start, Restart, a terminal, a worker) refuse it, so nothing can start in it. If the teardown fails it becomes `stopped` (`failed` if it was); a server that stops mid-delete finds it `failed`.

A workspace has no local directory. A session's environment, `local` or `docker`, is chosen when it is created (a Local/Docker select next to "New session") and recorded on the session. A local session clones its repo with `gh repo clone owner/name ~/.local/share/hypercube/sessions/<session-id>`; a Docker session clones into `/workspace` in its container (see "Docker"). The path is recorded before setup starts, so failed clones can also be deleted. Clones and containers stay across server restarts.

`Environment.setup(repo, clonePath)` and `teardown(clonePath)` own clone creation and removal; `ensureRunning()` makes sure what the session runs in is up (a no-op locally; `docker start` for a stopped container); `hostUrl(url)` is a URL of this server as a process in the environment reaches it (the same locally; `host.docker.internal` in a container). `spawn(cmd, cwd, env)` returns piped stdio and an exit promise; local processes are spawned in their own process group. `run(cmd, cwd?)` captures output for one-off commands and throws on failure. `spawnPty(cmd, cwd, { cols, rows, onData })` runs a command on a PTY and returns a `PtyProcess` (`write`, `resize`, `hangup`, `exited`). Both kinds of process are `Stoppable`: `stop()` stops the process and everything it owns gracefully (SIGTERM, then SIGKILL after a grace period) and resolves once they are gone, and calling it again returns the same promise; `stopNow()` sends SIGKILL to the same set synchronously, for server exit. Whoever started a process owns it until `stop()` settles. `resolveDir(env, clonePath, dir)` is the one clone-boundary check: it resolves `dir` (relative to the clone, or absolute) to a real path through `env.run` and throws unless the directory exists inside the clone. `loginShell()` is the command for an interactive login shell (`$SHELL -l` locally, `bash -l` in a container). `environmentFor(session)` returns the session's environment; the Docker one is per session, since it names the session's container.

## Docker

A Docker session (`src/server/docker.ts`) runs in one container named `hypercube-<session id>`, labelled `hypercube.session=<session id>`, from the image `hypercube-base:<hash>`. Nothing from the host's home directory is mounted.

- **Image.** Built from `docker/Dockerfile` (Debian with `git`, `gh`, `ripgrep`, `procps` and OpenCode, at the version the Dockerfile names; keep it in step with the host's) the first time a Docker session is set up. The tag is a hash of the Dockerfile and of the file `NODE_EXTRA_CA_CERTS` names, if the server has it set, which is copied into the image's CA store: a TLS-intercepting proxy on the host's network (Zscaler here) signs HTTPS with a CA containers don't know. Changing either builds a new image; otherwise it is reused.
- **`setup`** builds the image if missing, starts the container (`sleep infinity` under `--init`) with `GH_TOKEN` from the host's `gh auth token` (passed by name, so it isn't in `ps`), pipes `opencode auth export` on the host into `opencode auth import` in the container (through memory; no file, no argument; only the `openai` login is imported; if that fails the error is a fixed message, never `opencode`'s output, which could echo a credential), runs `gh auth setup-git`, copies the host's git `user.name` and `user.email` if it has them, and clones into `/workspace`. If any step fails the container is removed.
- **Processes.** `spawn` and `run` are `docker exec -i -w <cwd>`; `spawnPty` is `docker exec -it` on a local PTY. `docker exec` doesn't pass signals on, so each process is started as the leader of its own session (a PTY's already is one) and records its pid under `/run/hypercube`; `stop` signals every process in that session with SIGTERM and SIGHUP, then SIGKILL after two seconds, only while the process is still running, and waits for the command to finish; `stopNow` is the synchronous version. Background and reparented processes go with it; one that `setsid`s away is left, as for local terminals. If the container can't be reached, the docker CLI is stopped instead, which ends the process's stdin. Server shutdown (`SIGINT`, `SIGTERM`, `SIGHUP`) awaits `stop` for every agent and terminal before the process exits, since a command to the container can't be waited for while exiting; `process.exit` also runs `stopNow` as a last resort. Processes that OpenCode starts in sessions of its own (its shell commands, its `serve` daemon) are not in the agent's session and are left, as the Agents section says. `teardown` removes the container, which ends everything in it.
- **Safe delete** (`findUnsavedWork`) and `resolveDir` run `git` and `sh` in the container through `Environment.run`, as they do locally.
- **MCP.** Agents in a Docker session get `http://host.docker.internal:<MCP port>/mcp` (`Environment.hostUrl`); Docker Desktop makes that name reach the host's `127.0.0.1`, where the MCP listener is (see "Listeners"). `/mcp` accepts that `Host` as well as the loopback names, and still requires an agent's token and no `Origin` header. The app's port is not reachable from a container.
- **After Docker restarts.** Containers stay stopped. Each agent's start (`startAgent`: server startup, Start, Restart) calls `Environment.ensureRunning`, which runs `docker start` on a stopped container, before it spawns the process; the agent is `starting` meanwhile, so prompts sent to it queue. A container that no longer exists leaves the agent `exited` with "This session's container no longer exists. Delete the session." in its error; so does Docker being unreachable, with Docker's message. The session stays `ready`.

## Terminals

A session has at most one terminal: a login shell (`Environment.loginShell()`) in the session's clone, on a PTY from `Environment.spawnPty`. The terminal's id is its session's id. `Session.terminal` is true in the pushed model while it runs (see "Model").

**Lifecycle is explicit.** `startTerminal` (the "Start terminal" button in the bottom panel, which shows when the session is `ready` and has none) is the only thing that spawns a shell. It is killed by `closeTerminal` (the panel's ×), by stopping or deleting the session (a delete kills it before the clone is removed, and the session's `deleting` status refuses a new one meanwhile), by server shutdown, and when the shell exits by itself (`exit`): the terminal is forgotten once its last output has gone out and nothing it started is left, the model is pushed again, and the session shows the Start button again. A socket disconnect, page reload, HMR update or `bun --hot` reload never spawns or kills a shell: the registry, like the model, lives on `globalThis`, and a reload attaches again and gets a snapshot. Terminals are not stored; after a server restart no session has one.

**Ownership.** A terminal owns every process in its PTY's session, background, `nohup`ed and reparented ones included. A process that detaches into its own session (`setsid`, daemons) is no longer the terminal's and is left running. `stop` signals the session's members (SIGTERM and SIGHUP, then SIGKILL after 2 s for what is left), found by listing pids and keeping those whose `getsid` is the shell's pid. The kernel doesn't reuse a session id while it has members, and once the session has been seen empty its id is never looked up again, so a reused pid is never signalled. Closing the PTY also hangs up the foreground job. Server shutdown (`SIGINT`, `SIGTERM`, `SIGHUP`, handled in `src/index.ts`) closes every terminal, including ones mid-close, and waits before exiting; `process.exit` also runs `stopNow`, a synchronous SIGKILL of every terminal's members, as a last resort.

**Ordering.** Output from a PTY is batched (16 ms) and passed to a headless xterm per terminal; it is published to attached sockets from that parser's write callback, so it goes out in parse order. Snapshots and resizes go through the same write queue, so a snapshot contains exactly the output published before it and none after (the socket is subscribed in the same step it is sent a snapshot), and a resize applies after all earlier output, which was parsed at the old size. A shell that exits is forgotten only after its last output was published.

**Snapshots are best effort.** The serializer can't capture all terminal state, so after an attach the server takes the PTY one row off its size and back 50 ms later, which makes full-screen apps redraw. Nothing tries to make snapshots exact.

**In the browser.** `TerminalPanel` (the bottom panel) shows the active session's terminal in one xterm per workspace (`src/terminals.ts`, outside React so it keeps its screen), and switching sessions detaches from one terminal and attaches to the next. The view attaches once it has been fitted to its panel (so not while the panel is collapsed) and again on every reconnect. The panel never creates or closes a terminal on its own.

## Agents

When a session's clone is ready, `startAgent` registers the lead's runtime, makes sure the environment is up (`ensureRunning`), and spawns the lead through it (`opencode acp`, cwd = the clone, own process group) and talks ACP to it over its stdio with `@agentclientprotocol/sdk` (v1, server side only). It sends `initialize` advertising no client capabilities (no `fs`, no `terminal`), then `session/new` with the clone as `cwd` and Hypercube's MCP server (see below), and records the ACP session id on the agent. If the clone failed, the agent is `exited` with the session's error.

**Settings and profiles.** Right after `session/new`, `applySettings` sets the agent's model and then its effort (the model decides which efforts exist) through `session/set_config_option`: the config options with category `model` and `thought_level`, matched by value. The lead gets `lead` from the config, read when the lead starts; a worker gets its profile, read when `start_worker` is called. A value OpenCode doesn't offer, an option it doesn't have, or a failed set stops the agent: it ends `exited` and its error names the profile (or the lead settings) and the values OpenCode offers (the first 20). A config file that fails validation likewise leaves a lead `exited` with the file's error.

**Workers.** `startWorker` adds an agent with role `worker` to the lead's session and starts it the same way, with `cwd` = the directory the lead asked for and the profile's settings. The directory must exist and resolve, symlinks included, to inside the session's clone (`Environment.resolveDir`); a relative path is relative to the clone. Worker names are unique in the session, ignoring case, and can't be the lead's ("Lead"). A worker is the same as a lead in every other way: it has a tab, takes prompts, Stop and permission answers, and is steered by messages. A worker lives until `removeWorker` (the lead's `stop_worker`, or the user's remove action) or the session is deleted, and an exited worker stays until removed. Removing one deletes its transcript.

**Status.** `starting` from the moment `startAgent` is called until the ACP session exists, then `idle`. `busy` while a `session/prompt` is in flight, `waiting` while a permission request is open, back to `idle` when the prompt returns. `exited` when the process is gone or never started; `error` says why (a kill by us is not an error). An exited agent stays exited until the user restarts it (`restartAgent`, the Restart button in its panel) or its session is stopped and started; nothing restarts it on its own.

**Resume.** An agent that has an `acpSessionId` doesn't get a new ACP session when its process starts (after a server restart, Start, or Restart): `startAgent` sends `session/resume` with the recorded id, the agent's `cwd` and a fresh `mcpServers` entry (a new token, since the old process and its token are gone). If the agent doesn't advertise it or it fails, `session/load` follows, and its replay of the conversation is ignored, since the transcript is in the database. If that fails too, a new session is made, the transcript gets a notice saying the agent doesn't remember the earlier conversation, and the agent gets its role preamble again. The lead's or the worker's profile settings are applied again after any of the three; a worker whose profile has left the config ends `exited` with an error. A turn that was running when the old process stopped is lost: open permission requests in the transcript are closed as cancelled and unfinished tool calls marked cancelled. The preamble is sent again only if the agent was never prompted.

**Prompt queue.** One `session/prompt` per agent at a time. A prompt that arrives when the agent isn't idle (including while `starting`) waits in `agent.queue`, which is part of the model, and is sent in order when the turn ends. `cancel` sends `session/cancel`; the queue is kept. A steer that arrives when no turn is running (the agent is `starting`, or another queued prompt goes first) is queued with `steer` set, and turns into a real steer as soon as the next turn starts. The queue is not stored: prompts waiting at a stop or restart are lost.

**Steering.** A steer redirects an agent that is busy. OpenCode rejects a second `session/prompt` mid-turn ("Session already has an active ACP prompt"), so a steer interrupts the turn and becomes the next prompt. The user's prompt to a busy agent is a steer unless the panel queues it, and every `send_message` is one. A steer to an agent that isn't busy is an ordinary prompt. Otherwise it is added to `agent.steer` and any open permission request is answered `cancelled`. If a tool call is running (`in_progress`), the agent is let finish it: `agent.steer.waitingFor` names it, the panel shows it with Interrupt now (`interrupt`), and the next `tool_call_update` that leaves none running sends `session/cancel`. If none is (the model is thinking or writing, or a tool hasn't started), it cancels at once. When the cancelled prompt returns, the steer is sent before anything in the queue; the agent keeps the tool results that finished. Steers that arrive before the turn stops go out as one prompt. Its text says it was sent mid-turn and that the previous turn was interrupted after its last finished step (or, if the turn ended by itself first, that it had already ended), then gives each message with how long ago it was sent. The transcript shows each steer as its own `user` item with `steered` set. Stop keeps a pending steer, which goes out once the turn is cancelled. Verified with OpenCode only; another harness may lose the tool results on cancel.

**Transcripts.** The server folds `session/update`s into `TranscriptItem`s: `user` (the prompts we send; `from` names the sending agent when another agent sent it, and the panel shows it as a message from that agent), `agent` and `thought` (text chunks, joined by `messageId`), `tool` (title, kind, status; `tool_call_update` replaces the item for its `toolCallId`), `permission` (the request's options and the answer) and `notice` (a failed prompt, an unusual stop reason, the process exiting). Plans, usage, commands and config options go to the panel as the agent's `info` (see above), a whole snapshot each time, and aren't stored; they come back as the agent sends them. Transcripts are not in the model: they are sent as a `transcript` snapshot on connect and `item` upserts afterwards (see above). They are kept for every agent, running or not, in memory and in the database, and every upsert is written as it happens.

**Role preamble.** ACP has no system prompt, so an agent's first prompt is preceded by `leadPreamble` or `workerPreamble` (`agents.ts`): who it is, its session and workspace, its tools and the messaging rules. A worker's names its lead and its directory, and tells it to report to the lead with `send_message`. Both say that messages arrive mid-turn, so an agent doesn't end its turn to receive a reply. The transcript shows only what was sent, not the preamble.

**Lifetime.** Agent processes, connections and transcripts live on `globalThis`, so `bun --hot` reloads keep them. A socket disconnect or reload never starts or kills one. **Ownership.** An agent's runtime is registered when `startAgent` begins and stays owned until its process's `stop()` has settled, even after the agent is stopped or exits; a stopped agent's token stops working at once. `startAgent` first waits for a previous runtime of the same agent to finish stopping, so Restart, and Start right after Stop, never overlap two processes. `stopSession` and `deleteSession` stop the session's agents, workers included, and wait: SIGTERM to the process group, SIGKILL after 5 s (`deleteSession` then removes the clone). **Shutdown** is owned by `src/index.ts`, the only place that installs signal and `exit` handlers (once, calling the latest code across `bun --hot` reloads): `SIGINT`, `SIGTERM` and `SIGHUP` stop every terminal and every agent, including ones already being stopped, and wait before exiting, including, for Docker sessions, everything the agent started in its container's session; `process.exit` also runs `stopNow` (SIGKILL) on all of them as a last resort. The sessions stay `ready` in the database, so the next start resumes their agents. OpenCode starts its shell commands in process groups of their own, so the group kill doesn't reach them directly: they stop because OpenCode stops them on SIGTERM. Ownership of such children is not handled beyond that.

## Persistence

The model and the transcripts are stored in SQLite (`bun:sqlite`) at `~/.local/share/hypercube/hypercube.db`, or at `$HYPERCUBE_DB` when that is set (a test instance uses its own file). `src/server/store.ts` has tables `workspaces`, `sessions`, `agents` and `items`, each row one record as JSON. There are no migrations: a database whose `user_version` isn't the code's is dropped and recreated, so a schema change loses the model (and leaves the clones on disk).

- **Writes.** The model is written when it is pushed (after each change, once per tick): each record is upserted, only rows whose JSON changed are written, and rows for records no longer in the model are deleted. An agent is stored without its `status`, `error`, `queue` and `steer`, which belong to a running server. Transcript items are written one at a time as they change, in place, so their order is kept; the same transaction that writes the model deletes the items of agents no longer in it. A failed write is logged and doesn't stop the server. Nothing is written until `restoreModel` has opened the database, so tests and a hot-reloaded server from before the database don't touch a file.
- **Startup.** `restoreModel` (called from `src/index.ts` once the server is bound, once per process) loads the model and every transcript. A session that was `cloning` becomes `failed`. A session that was `deleting` becomes `failed` too. For each `ready` session, every agent, workers included, becomes `starting` and is started by `startAgent`, which brings the session's environment up first (`ensureRunning`; a stopped Docker container is started) and resumes it as described under "Agents"; stopped and failed sessions' agents stay `exited`. All of a server's agents start at once.
- **Not stored:** tokens, queues, pending steers, and panel info (plan, usage, config options, commands). Tokens exist only in a running agent's runtime state.

## Deleting a session

`findUnsavedWork` (`src/server/unsaved.ts`, through `Environment.run`) looks at the clone and at every `git worktree` that resolves to inside it (worktrees elsewhere aren't removed with the clone, so they aren't checked). In each it runs `git status --porcelain`, and lists commits reachable from `HEAD` that are on no remote-tracking branch (`git log HEAD --not --remotes`; for the clone also from every local branch, which the worktrees share, so a commit is listed once). A worktree shows up in the clone's own status as an untracked directory; that line is left out, since the worktree is checked itself. Ignored files and stashes aren't looked at. If a git command fails, the result is one entry with `error` set, so the user still gets to confirm. A clone that can't be reached (gone, or its container is stopped) is not taken to have nothing to lose: it is the same one entry with `error`, so the user is asked. The check runs before anything is stopped, so a refused delete changes nothing; work an agent writes between the check and the kill isn't caught.

## MCP server

`session/new` (and `session/resume` or `session/load`) carries one `http` MCP server named `hypercube`, pointing at `POST /mcp` on the MCP listener (see "Listeners"; `host.docker.internal` for a Docker session), with an `Authorization: Bearer <token>` header. The token is a random UUID made each time the agent's process starts (a resumed agent gets a new one) and kept only in the agent's runtime state; it stops working when the agent is stopped or deleted. The endpoint maps the token to the agent and rejects everything else (see the routes table). It speaks the minimum of MCP over streamable HTTP: one JSON-RPC message per POST, answered with plain JSON (`initialize`, `ping`, `tools/list`, `tools/call`; notifications get `202`), no server-to-client stream, no session id.

Tools (a worker's `tools/list` leaves out the lead-only ones, and calling them is refused too):

- `list_agents`: the agents the caller may message, as `{ id, name, role, session, status, profile?, error? }`. `error` is why an exited agent exited.
- `send_message(to, text)`: `to` is an agent id or a name. A name shared by several agents is refused with their ids (every lead is named "Lead"). The message is delivered to the recipient as a steer (see "Steering"): if it is working, its turn is interrupted after its current tool call. It reaches the agent as `Message from <name> (<session>) (id <id>, sent <time> ago).` followed by the text. A failure the agent can act on (no such agent, out of scope, recipient exited) comes back as a tool result with `isError`.

- `list_profiles`: the profiles from the config, as `{ name, description, model, effort }`.
- `start_worker(name, profile, dir, message?)`: lead only. Starts a worker (see above) and, if `message` is given, queues it as the worker's first prompt from the lead. Returns the worker's id. Unknown profile, bad directory and duplicate name come back as `isError` results. A worker that fails later (a profile OpenCode can't satisfy) shows up as `exited` with its `error` in `list_agents`.
- `stop_worker(id)`: lead only. `id` is a worker's id or name in the lead's own session. Removes it as `removeWorker` does.

Scopes: a lead may message its own workers and the other leads in its workspace; a worker may message only its lead, so nothing leaves its session except through the lead. `list_agents` shows the same set. Messages don't wait for the recipient's turn to end.

## Config

`~/.config/hypercube/config.json`, or `$XDG_CONFIG_HOME/hypercube/config.json` when that is set, read on every use (`src/server/config.ts`). It shares its shape with the Rust Hypercube's file (`~/projects/hypercube/docs/configuration.md`) and may be that file:

```json
{
  "lead": { "harness": "opencode", "model": "openai/gpt-6-luna", "effort": "high" },
  "profiles": [{ "name": "luna", "description": "When to use it", "harness": "opencode", "model": "openai/gpt-6-luna", "effort": "high" }]
}
```

`model` and `effort` are values OpenCode offers for its `model` and `thought_level` options. Without the file, the built-in default is the lead above and the profiles `luna` (`openai/gpt-6-luna`, high), `sol` (`openai/gpt-6.1-sol`, high) and `terra` (`openai/gpt-6.1-terra`, medium). Hypercube never writes the file. A file that exists must be valid: a JSON object, a non-empty `profiles` array, each profile with a `name` (unique), `description`, `harness` (`claude` or `opencode`), `model` and `effort`; `lead`, if present, the same without name and description and with harness `opencode`. Errors name the file and the profile. Unlike the Rust file, `lead` is optional (the built-in lead is used without it, so a file that only has `orchestrator` works), and profiles for harnesses other than `opencode` are accepted and not offered, since only OpenCode runs. Other fields (`orchestrator`) are ignored.

## Browser-only state

What the user is looking at is not in the model and resets on page reload: the active workspace and whether the overview is shown (`App`), the active session (`WorkspaceView`), and which side panels are open. Workspaces and sessions mount on first visit and stay mounted while the tab is open.

**Status in the UI.** An agent with status `waiting` has an open permission request and needs the user. It shows as an amber marker on its tab (tabs carry no other status), "Waiting on you: names" on its session's row, an amber ring and count on its workspace's overview card, and a dot on the workspace header's overview button when any agent anywhere waits. Busy agents show as "Busy: names" on the row and a count on the card. The right panel lists the active session's agents with role, profile, status and queue and steer counts; selecting one shows its tab. The bottom panel is the terminal (see "Terminals").
