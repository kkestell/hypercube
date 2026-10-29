import type { Server, ServerWebSocket, WebSocketHandler } from "bun";
import { FRAME_OUTPUT, FRAME_SNAPSHOT, encodeFrame, type ClientMessage, type ModelRequest, type ServerMessage } from "../protocol";
import { answerPermission, bindAgents, cancelAgent, getInfos, getTranscripts, interruptAgent, promptAgent, setConfigOption } from "./agents";
import { ModelError, UnsavedWorkError } from "./errors";
import { addWorkspace, bindModel, closeSessionTerminal, createSession, deleteSession, flushModelChange, getModel, removeWorker, restartAgent, startSession, startTerminal, stopSession } from "./sessions";
import { attachTerminal, getTerminal, resizeTerminal, setHooks, writeInput, type TerminalEntry } from "./terminals";

/**
 * The browser's WebSocket: one per tab. It pushes the model to every client
 * and carries their requests to change it, and the input and output of the
 * terminals the tab shows. Each terminal is a pub/sub topic, so a client that
 * has not attached (or has detached) is sent none of its output. Closing a
 * socket never touches terminals; they outlive it.
 */

const MODEL_TOPIC = "model";
const ITEMS_TOPIC = "items";

export type SocketData = {
  /** Terminals this client is attached to. */
  attached: Set<string>;
};

const MAX_ID_BYTES = 200;
const topicOf = (id: string) => `terminal:${id}`;

// Shared across `bun --hot` reloads, like the terminal registry.
const globals = globalThis as typeof globalThis & { __hypercubeSockets?: Set<ServerWebSocket<SocketData>> };
const sockets = (globals.__hypercubeSockets ??= new Set());

export const LOOPBACK_HOSTNAMES = new Set(["localhost", "127.0.0.1", "[::1]"]);
const LOOPBACK_ADDRESSES = new Set(["127.0.0.1", "::1", "::ffff:127.0.0.1"]);

/**
 * Whether a request comes from this machine and names a loopback host on the
 * server's own port. The Host check stops DNS rebinding, where a page on
 * another name resolves to this machine.
 */
export function isLoopbackRequest(req: Request, port: number, peerAddress: string | undefined, hostnames = LOOPBACK_HOSTNAMES) {
  if (!peerAddress || !LOOPBACK_ADDRESSES.has(peerAddress)) return false;
  const host = req.headers.get("host")?.toLowerCase();
  if (!host) return false;
  let url: URL;
  try {
    url = new URL(`http://${host}`);
  } catch {
    return false;
  }
  return url.host === host && hostnames.has(url.hostname) && Number(url.port || 80) === port;
}

/**
 * Whether a WebSocket upgrade may be accepted: a loopback request that also
 * comes from a page served by exactly this origin.
 */
export function isAllowedUpgrade(req: Request, port: number, peerAddress: string | undefined) {
  if (!isLoopbackRequest(req, port, peerAddress)) return false;
  const host = req.headers.get("host")!.toLowerCase();
  return req.headers.get("origin")?.toLowerCase() === `http://${host}`;
}

export function upgradeSocket(req: Request, server: Server<SocketData>) {
  if (!isAllowedUpgrade(req, server.port!, server.requestIP(req)?.address)) {
    return new Response("Forbidden", { status: 403 });
  }
  if (server.upgrade(req, { data: { attached: new Set() } satisfies SocketData })) return;
  return new Response("Expected a WebSocket upgrade", { status: 400 });
}

function sendJson(ws: ServerWebSocket<SocketData>, message: ServerMessage) {
  ws.send(JSON.stringify(message));
}

/**
 * Pushes model, transcript and terminal changes to clients. Call once with the
 * app's server, again after each hot reload. `mcpUrl` is where agents reach
 * Hypercube's MCP server, which is not this server (see `src/index.ts`).
 */
export function bindServer(server: Server<SocketData>, mcpUrl: string) {
  setHooks({
    output: (entry, chunk) => server.publish(topicOf(entry.id), encodeFrame(FRAME_OUTPUT, entry.id, chunk)),
    removed(id) {
      for (const ws of sockets) {
        ws.unsubscribe(topicOf(id));
        ws.data.attached.delete(id);
      }
    },
  });
  bindModel({
    onChange: model => server.publish(MODEL_TOPIC, JSON.stringify({ t: "model", model } satisfies ServerMessage)),
  });
  bindAgents({
    mcpUrl,
    onItem: (agentId, item) => server.publish(ITEMS_TOPIC, JSON.stringify({ t: "item", agentId, item } satisfies ServerMessage)),
    onInfo: (agentId, info) => server.publish(ITEMS_TOPIC, JSON.stringify({ t: "info", agentId, info } satisfies ServerMessage)),
  });
}

const modelRequests: { [K in ModelRequest["t"]]: (request: Extract<ModelRequest, { t: K }>) => string | void | Promise<string | void> } = {
  addWorkspace: ({ repo }) => addWorkspace(String(repo)),
  createSession: ({ workspaceId, environment }) => createSession(String(workspaceId), environment),
  deleteSession: ({ sessionId, confirmed }) => deleteSession(String(sessionId), confirmed === true),
  stopSession: ({ sessionId }) => stopSession(String(sessionId)),
  startSession: ({ sessionId }) => startSession(String(sessionId)),
  restartAgent: ({ agentId }) => restartAgent(String(agentId)),
  removeWorker: ({ agentId }) => removeWorker(String(agentId)),
  startTerminal: ({ sessionId }) => startTerminal(String(sessionId)),
  closeTerminal: ({ sessionId }) => closeSessionTerminal(String(sessionId)),
  prompt: ({ agentId, text, queue }) => promptAgent(String(agentId), String(text), undefined, queue === true ? "queue" : "steer"),
  interrupt: ({ agentId }) => interruptAgent(String(agentId)),
  cancel: ({ agentId }) => cancelAgent(String(agentId)),
  answerPermission: ({ agentId, requestId, optionId }) => answerPermission(String(agentId), String(requestId), String(optionId)),
  setConfigOption: ({ agentId, configId, value }) => setConfigOption(String(agentId), String(configId), String(value)),
};

/** Sends the terminal's screen to the client at `size`, then subscribes it to live output. */
function attachClient(ws: ServerWebSocket<SocketData>, entry: TerminalEntry, size: { cols: number; rows: number }) {
  const { id } = entry;
  ws.data.attached.add(id);
  ws.unsubscribe(topicOf(id));
  attachTerminal(entry, size, snapshot => {
    // The client may have detached, or the socket closed, while the snapshot was prepared.
    if (ws.readyState !== 1 || !ws.data.attached.has(id)) return;
    ws.send(encodeFrame(FRAME_SNAPSHOT, id, snapshot));
    ws.subscribe(topicOf(id));
  });
}

const clampSize = (value: unknown, fallback: number) => {
  const size = Math.floor(Number(value));
  return Number.isFinite(size) && size > 0 ? Math.min(size, 1000) : fallback;
};

function handleTerminalMessage(ws: ServerWebSocket<SocketData>, message: Extract<ClientMessage, { id: string }>) {
  const { id } = message;
  if (typeof id !== "string" || id.length === 0 || new TextEncoder().encode(id).length > MAX_ID_BYTES) return;

  switch (message.t) {
    case "attach": {
      // A terminal that is gone is ignored: the model, which says whether a session has one, is the source of truth.
      const entry = getTerminal(id);
      if (entry) attachClient(ws, entry, { cols: clampSize(message.cols, 80), rows: clampSize(message.rows, 24) });
      return;
    }

    case "detach":
      ws.data.attached.delete(id);
      ws.unsubscribe(topicOf(id));
      return;

    case "input":
      if (typeof message.data === "string") writeInput(id, message.data);
      return;

    case "resize":
      return resizeTerminal(id, clampSize(message.cols, 80), clampSize(message.rows, 24));
  }
}

const terminalMessages = new Set(["attach", "detach", "input", "resize"]);

async function handleMessage(ws: ServerWebSocket<SocketData>, input: ClientMessage) {
  if (typeof input.t !== "string") return;
  if (terminalMessages.has(input.t)) return handleTerminalMessage(ws, input as Extract<ClientMessage, { id: string }>);
  if (!Object.hasOwn(modelRequests, input.t)) return;
  const message = input as ModelRequest & { req: number };
  const req = Number(message.req);
  const handler = modelRequests[message.t] as (request: ModelRequest) => string | void | Promise<string | void>;
  try {
    const id = await handler(message);
    flushModelChange();
    sendJson(ws, { t: "result", req, ok: true, id: id ?? undefined });
  } catch (error) {
    if (!(error instanceof ModelError)) console.error(`Request ${message.t} failed:`, error);
    sendJson(ws, { t: "result", req, ok: false, error: error instanceof Error ? error.message : String(error), unsaved: error instanceof UnsavedWorkError ? error.unsaved : undefined });
  }
}

export const socket: WebSocketHandler<SocketData> = {
  open(ws) {
    sockets.add(ws);
    ws.subscribe(MODEL_TOPIC);
    ws.subscribe(ITEMS_TOPIC);
    sendJson(ws, { t: "model", model: getModel() });
    for (const { agentId, items } of getTranscripts()) sendJson(ws, { t: "transcript", agentId, items });
    for (const { agentId, info } of getInfos()) sendJson(ws, { t: "info", agentId, info });
  },

  async message(ws, message) {
    if (typeof message !== "string") return;
    try {
      await handleMessage(ws, JSON.parse(message));
    } catch (error) {
      console.error("Bad message:", error);
    }
  },

  close(ws) {
    sockets.delete(ws);
  },
};
