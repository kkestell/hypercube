import type { Server, ServerWebSocket, WebSocketHandler } from "bun";
import type { ClientMessage, ModelRequest, ServerMessage } from "../protocol";
import { ModelError, addWorkspace, bindModel, createSession, flushModelChange, getModel } from "./sessions";

/**
 * The browser's WebSocket: one per tab. It pushes the model to every client
 * and carries their requests to change it.
 */

const MODEL_TOPIC = "model";

export type SocketData = Record<string, never>;

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
  if (server.upgrade(req, { data: {} })) return;
  return new Response("Expected a WebSocket upgrade", { status: 400 });
}

function sendJson(ws: ServerWebSocket<SocketData>, message: ServerMessage) {
  ws.send(JSON.stringify(message));
}

/** Pushes model changes to clients. Call once with the server, again after each hot reload. */
export function bindServer(server: Server<SocketData>) {
  bindModel({
    onChange: model => server.publish(MODEL_TOPIC, JSON.stringify({ t: "model", model } satisfies ServerMessage)),
  });
}

const modelRequests: { [K in ModelRequest["t"]]: (request: Extract<ModelRequest, { t: K }>) => string } = {
  addWorkspace: ({ path }) => addWorkspace(String(path)),
  createSession: ({ workspaceId }) => createSession(String(workspaceId)),
};

function handleMessage(ws: ServerWebSocket<SocketData>, message: ClientMessage) {
  if (typeof message.t !== "string" || !Object.hasOwn(modelRequests, message.t)) return;
  const req = Number(message.req);
  const handler = modelRequests[message.t] as (request: ModelRequest) => string;
  try {
    const id = handler(message);
    flushModelChange();
    sendJson(ws, { t: "result", req, ok: true, id });
  } catch (error) {
    if (!(error instanceof ModelError)) console.error(`Request ${message.t} failed:`, error);
    sendJson(ws, { t: "result", req, ok: false, error: error instanceof Error ? error.message : String(error) });
  }
}

export const socket: WebSocketHandler<SocketData> = {
  open(ws) {
    ws.subscribe(MODEL_TOPIC);
    sendJson(ws, { t: "model", model: getModel() });
  },

  message(ws, message) {
    if (typeof message !== "string") return;
    try {
      handleMessage(ws, JSON.parse(message));
    } catch (error) {
      console.error("Bad message:", error);
    }
  },
};
