import type { ModelRequest, ServerMessage } from "./protocol";
import { applyModel } from "./modelStore";

/**
 * The tab's single WebSocket to the server: the model it pushes and requests
 * to change it. It reconnects on its own.
 */

const RECONNECT_MIN_MS = 250;
const RECONNECT_MAX_MS = 5000;

const pending = new Map<number, { resolve: (id: string | undefined) => void; reject: (error: Error) => void }>();
let nextRequest = 1;
/** The tab's one socket, open or connecting. */
let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectDelay = RECONNECT_MIN_MS;

export function connect() {
  if (socket) return;
  if (reconnectTimer) clearTimeout(reconnectTimer);
  reconnectTimer = null;

  const url = new URL("/ws", location.href);
  url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(url);
  socket = ws;

  ws.onopen = () => {
    if (socket !== ws) return;
    reconnectDelay = RECONNECT_MIN_MS;
  };

  ws.onmessage = event => {
    if (socket !== ws) return;
    const message: ServerMessage = JSON.parse(event.data);
    switch (message.t) {
      case "model":
        applyModel(message.model);
        break;
      case "result": {
        const request = pending.get(message.req);
        pending.delete(message.req);
        if (message.ok) request?.resolve(message.id);
        else request?.reject(new Error(message.error));
        break;
      }
    }
  };

  ws.onclose = () => {
    if (socket !== ws) return;
    socket = null;
    for (const request of pending.values()) request.reject(new Error("Lost the connection to the server"));
    pending.clear();
    reconnectTimer = setTimeout(connect, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
  };
}

/** Asks the server to change the model. Resolves with the new object's id. */
export function request(message: ModelRequest): Promise<string | undefined> {
  if (socket?.readyState !== WebSocket.OPEN) return Promise.reject(new Error("Not connected to the server"));
  const req = nextRequest++;
  return new Promise((resolve, reject) => {
    pending.set(req, { resolve, reject });
    socket!.send(JSON.stringify({ ...message, req }));
  });
}
