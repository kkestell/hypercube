# Architecture

Hypercube is a Bun server and a React app served by it. The server owns the state; the browser displays it and asks for changes over one WebSocket.

```
Browser tab                                   Bun server (src/index.ts)
───────────                                   ─────────────────────────
React UI ── useModel() ── modelStore          GET /*   → index.html (bundled React app, HMR in dev)
                              ▲               GET /ws  → WebSocket upgrade (loopback + Origin check)
                              │ model
src/socket.ts ◀═══════ one WebSocket ═══════▶ src/server/socket.ts ── src/server/sessions.ts (model)
     request() ──── addWorkspace / createSession ────▶
```

## Files

| File | Role |
|---|---|
| `src/index.ts` | `Bun.serve`: serves the app, upgrades `/ws`, binds to `localhost`. |
| `src/model.ts` | Model types, shared by server and browser. |
| `src/protocol.ts` | WebSocket message types, shared by server and browser. |
| `src/server/sessions.ts` | The model and its mutations (`addWorkspace`, `createSession`). |
| `src/server/socket.ts` | Upgrade check, pushes the model, dispatches requests. |
| `src/socket.ts` | The tab's socket: connects, reconnects, `request()`. |
| `src/modelStore.ts` | The last model received; `useModel()` for components. |
| `App.tsx`, `WorkspaceView.tsx`, `SessionList.tsx`, `SessionView.tsx`, `AgentPanel.tsx` | UI. |

## HTTP routes

| Route | Purpose |
|---|---|
| `GET /ws` | WebSocket upgrade. Returns 403 unless the request is from a loopback address, `Host` is `localhost`, `127.0.0.1` or `[::1]` on the server's port, and `Origin` is exactly `http://<Host>`. |
| `GET /*` | The React app (`index.html`). |

There are no other HTTP endpoints. All app traffic goes over the WebSocket.

## WebSocket protocol

Every message is a JSON text frame. Types are in `src/protocol.ts`.

**Server → browser**

| Message | When |
|---|---|
| `{ t: "model", model }` | On connect, and after every change. Always the whole model; the browser replaces its copy. |
| `{ t: "result", req, ok: true, id? }` | Reply to a request. `id` is the created workspace or session. |
| `{ t: "result", req, ok: false, error }` | A request failed; `error` is shown to the user. |

**Browser → server**

| Message | Effect |
|---|---|
| `{ t: "addWorkspace", path, req }` | Expands `~`, resolves the real path, requires an existing directory not already open. Name is the directory's basename. |
| `{ t: "createSession", workspaceId, req }` | Creates "Untitled N" (N counts up per workspace, never reused) with one agent, the lead. |

`req` is a number the browser picks per request; the matching `result` carries it back. A change's `model` push is sent before its `result`, so when `request()` resolves the new object is already in the store. Pushes are coalesced: several changes in one tick send one `model`. Unknown or malformed messages are ignored.

The browser's `request()` rejects if the socket isn't open, and all pending requests reject if it drops. The socket reconnects on its own (250 ms backoff doubling to 5 s) and gets the full model again on reconnect. A tab never has more than one socket open or connecting.

## Model

```ts
Model     = { version, workspaces: Workspace[], sessions: Record<id, Session>, agents: Record<id, Agent> }
Workspace = { id, name, path /* realpath */, nextSessionNumber, sessionIds }
Session   = { id, workspaceId, name, agentIds, createdAt }
Agent     = { id, sessionId, role: "lead" | "worker", name }
```

The model lives in server memory on `globalThis`, so it survives page reloads and `bun --hot` reloads. It is not persisted: restarting the server starts empty. Agents are records only; nothing runs.

## Browser-only state

What the user is looking at is not in the model and resets on page reload: the active workspace and whether the overview is shown (`App`), the active session (`WorkspaceView`), and which side panels are open. Workspaces and sessions mount on first visit and stay mounted while the tab is open.
