import type { Server, ServerWebSocket, WebSocketHandler } from "bun";
import {
  FRAME_OUTPUT,
  FRAME_SNAPSHOT,
  encodeFrame,
  type ClientMessage,
  type ModelRequest,
  type ServerMessage,
  type TerminalStatus,
} from "../protocol";
import {
  ModelError,
  addWorkspace,
  bindModel,
  createSession,
  flushModelChange,
  getModel,
  restartShell,
  retryAgent,
  startSession,
  stopSession,
} from "./sessions";
import {
  attachTerminal,
  getTerminal,
  listTerminals,
  resizeTerminal,
  setHooks,
  statusOf,
  writeInput,
  type TerminalEntry,
} from "./terminals";

/**
 * The multiplexed terminal WebSocket: one per browser tab, carrying every
 * terminal the tab has open. Each terminal is a pub/sub topic, so a client
 * that has not attached (or has detached) is sent none of its output. Every
 * client is also subscribed to a low-rate status topic covering all terminals.
 */

const STATUS_TOPIC = "status";
const MODEL_TOPIC = "model";
const STATUS_INTERVAL_MS = 250;
/** A client with this much unsent output is dropped from the live stream and resynced with a snapshot. */
const STALL_BYTES = 4 * 1024 * 1024;
const MAX_ID_BYTES = 200;

export type SocketData = {
  /** Terminals this client is attached to. */
  attached: Set<string>;
  /** Attached terminals whose live stream was cut because the client fell behind. */
  stalled: Set<string>;
};

const topicOf = (id: string) => `terminal:${id}`;

// Shared across `bun --hot` reloads, like the terminal registry.
const globals = globalThis as typeof globalThis & { __hypercubeSockets?: Set<ServerWebSocket<SocketData>> };
const sockets = (globals.__hypercubeSockets ??= new Set());

const LOOPBACK_HOSTNAMES = new Set(["localhost", "127.0.0.1", "[::1]"]);
const LOOPBACK_ADDRESSES = new Set(["127.0.0.1", "::1", "::ffff:127.0.0.1"]);

/**
 * Whether a request comes from this machine and names a loopback host on the
 * server's own port. The Host check stops DNS rebinding, where a page on
 * another name resolves to this machine.
 */
export function isLoopbackRequest(req: Request, port: number, peerAddress: string | undefined) {
  if (!peerAddress || !LOOPBACK_ADDRESSES.has(peerAddress)) return false;
  const host = req.headers.get("host")?.toLowerCase();
  if (!host) return false;
  let url: URL;
  try {
    url = new URL(`http://${host}`);
  } catch {
    return false;
  }
  return url.host === host && LOOPBACK_HOSTNAMES.has(url.hostname) && Number(url.port || 80) === port;
}

/**
 * Whether a WebSocket upgrade may open a shell: a loopback request that also
 * comes from a page served by exactly this origin.
 */
export function isAllowedUpgrade(req: Request, port: number, peerAddress: string | undefined) {
  if (!isLoopbackRequest(req, port, peerAddress)) return false;
  const host = req.headers.get("host")!.toLowerCase();
  return req.headers.get("origin")?.toLowerCase() === `http://${host}`;
}

export function upgradeTerminalSocket(req: Request, server: Server<SocketData>) {
  if (!isAllowedUpgrade(req, server.port!, server.requestIP(req)?.address)) {
    return new Response("Forbidden", { status: 403 });
  }
  const data: SocketData = { attached: new Set(), stalled: new Set() };
  if (server.upgrade(req, { data })) return;
  return new Response("Expected a WebSocket upgrade", { status: 400 });
}

function sendJson(ws: ServerWebSocket<SocketData>, message: ServerMessage) {
  ws.send(JSON.stringify(message));
}

function fullStatus(): Record<string, TerminalStatus> {
  return Object.fromEntries(listTerminals().map(entry => [entry.id, statusOf(entry)]));
}

/** Wires terminal events to clients. Call once with the server, again after each hot reload. */
export function bindTerminalServer(server: Server<SocketData>) {
  const changed = new Set<string>();
  let statusTimer: ReturnType<typeof setTimeout> | null = null;

  const publishStatus = () => {
    statusTimer = null;
    const terminals: Record<string, TerminalStatus> = {};
    const removed: string[] = [];
    for (const id of changed) {
      const entry = getTerminal(id);
      if (entry) terminals[id] = statusOf(entry);
      else removed.push(id);
    }
    changed.clear();
    const message: ServerMessage = { t: "status", terminals, ...(removed.length && { removed }) };
    server.publish(STATUS_TOPIC, JSON.stringify(message));
  };

  bindModel({
    url: `http://localhost:${server.port}`,
    onChange: model => server.publish(MODEL_TOPIC, JSON.stringify({ t: "model", model } satisfies ServerMessage)),
  });

  setHooks({
    output(entry, chunk) {
      server.publish(topicOf(entry.id), encodeFrame(FRAME_OUTPUT, entry.id, chunk));
      // publish() queues for slow clients without limit, so stop feeding any that fall too far behind.
      for (const ws of sockets) {
        if (ws.data.attached.has(entry.id) && !ws.data.stalled.has(entry.id) && ws.getBufferedAmount() > STALL_BYTES) {
          ws.unsubscribe(topicOf(entry.id));
          ws.data.stalled.add(entry.id);
        }
      }
    },

    exit(entry) {
      const message: ServerMessage = { t: "exit", id: entry.id, exitCode: entry.exitCode };
      server.publish(topicOf(entry.id), JSON.stringify(message));
    },

    changed(id) {
      changed.add(id);
      statusTimer ??= setTimeout(publishStatus, STATUS_INTERVAL_MS);
    },

    removed(id) {
      for (const ws of sockets) {
        ws.unsubscribe(topicOf(id));
        ws.data.attached.delete(id);
        ws.data.stalled.delete(id);
      }
    },
  });
}

/** Sends the terminal's screen to the client, at `size` if given, then subscribes it to live output. */
function attachClient(ws: ServerWebSocket<SocketData>, entry: TerminalEntry, size: { cols: number; rows: number } | null) {
  const { id } = entry;
  ws.data.attached.add(id);
  ws.data.stalled.delete(id);
  ws.unsubscribe(topicOf(id));
  attachTerminal(entry, size, snapshot => {
    // The client may have detached while the snapshot was prepared.
    if (ws.readyState !== 1 || !ws.data.attached.has(id)) return;
    ws.send(encodeFrame(FRAME_SNAPSHOT, id, snapshot));
    ws.subscribe(topicOf(id));
    // Only once the exit has been published: until then it reaches this client through its subscription, after the last output.
    if (entry.exitPublished) sendJson(ws, { t: "exit", id, exitCode: entry.exitCode });
  });
}

const clampSize = (value: unknown, fallback: number) => {
  const size = Math.floor(Number(value));
  return Number.isFinite(size) && size > 0 ? Math.min(size, 1000) : fallback;
};

const modelRequests: { [K in ModelRequest["t"]]: (request: Extract<ModelRequest, { t: K }>) => string | void } = {
  addWorkspace: ({ path }) => addWorkspace(String(path)),
  createSession: ({ workspaceId, harness }) => createSession(String(workspaceId), harness),
  startSession: ({ sessionId }) => startSession(String(sessionId)),
  stopSession: ({ sessionId }) => stopSession(String(sessionId)),
  retryAgent: ({ agentId }) => retryAgent(String(agentId)),
  restartShell: ({ workspaceId }) => restartShell(String(workspaceId)),
};

function handleModelRequest(ws: ServerWebSocket<SocketData>, message: ModelRequest & { req: number }) {
  const req = Number(message.req);
  const handler = modelRequests[message.t] as (request: ModelRequest) => string | void;
  try {
    const id = handler(message);
    flushModelChange();
    sendJson(ws, { t: "result", req, ok: true, ...(id && { id }) });
  } catch (error) {
    if (!(error instanceof ModelError)) console.error(`Request ${message.t} failed:`, error);
    sendJson(ws, { t: "result", req, ok: false, error: error instanceof Error ? error.message : String(error) });
  }
}

function handleMessage(ws: ServerWebSocket<SocketData>, message: ClientMessage) {
  if (typeof message.t === "string" && Object.hasOwn(modelRequests, message.t)) return handleModelRequest(ws, message as ModelRequest & { req: number });
  if (!("id" in message)) return;
  const { id } = message;
  if (typeof id !== "string" || id.length === 0 || new TextEncoder().encode(id).length > MAX_ID_BYTES) return;

  switch (message.t) {
    case "attach": {
      const entry = getTerminal(id);
      if (!entry) return sendJson(ws, { t: "gone", id });
      return attachClient(ws, entry, { cols: clampSize(message.cols, 80), rows: clampSize(message.rows, 24) });
    }

    case "detach":
      ws.data.attached.delete(id);
      ws.data.stalled.delete(id);
      ws.unsubscribe(topicOf(id));
      return;

    case "input":
      if (typeof message.data === "string") writeInput(id, message.data);
      return;

    case "resize":
      return resizeTerminal(id, clampSize(message.cols, 80), clampSize(message.rows, 24));
  }
}

export const terminalSocket: WebSocketHandler<SocketData> = {
  open(ws) {
    sockets.add(ws);
    ws.subscribe(STATUS_TOPIC);
    ws.subscribe(MODEL_TOPIC);
    sendJson(ws, { t: "status", full: true, terminals: fullStatus() });
    sendJson(ws, { t: "model", model: getModel() });
  },

  message(ws, message) {
    if (typeof message !== "string") return;
    try {
      handleMessage(ws, JSON.parse(message));
    } catch (error) {
      console.error("Bad terminal message:", error);
    }
  },

  // Called when the client has caught up; resync the terminals it fell behind on.
  drain(ws) {
    if (ws.getBufferedAmount() > 0) return;
    for (const id of ws.data.stalled) {
      const entry = getTerminal(id);
      ws.data.stalled.delete(id);
      if (entry) attachClient(ws, entry, null);
    }
  },

  // Closing a socket never touches terminals; they outlive it.
  close(ws) {
    sockets.delete(ws);
  },
};
