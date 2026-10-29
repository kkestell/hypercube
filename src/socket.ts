import type { UnsavedWork } from "./model";
import { FRAME_OUTPUT, FRAME_SNAPSHOT, decodeFrame, type ClientMessage, type ModelRequest, type ServerMessage, type TerminalMessage } from "./protocol";
import { applyModel } from "./modelStore";
import { clearTranscripts, mergeInfo, setTranscript, upsertItem } from "./transcriptStore";

/**
 * The tab's single WebSocket to the server: the model and transcripts it pushes,
 * requests to change them, and the terminals this tab shows. It reconnects on
 * its own; terminal views reattach in `onOpen`.
 */

export type TerminalChannel = {
  /** The socket just opened (first time or after a reconnect). */
  onOpen(): void;
  onSnapshot(snapshot: Uint8Array): void;
  onOutput(chunk: Uint8Array): void;
};

const RECONNECT_MIN_MS = 250;
const RECONNECT_MAX_MS = 5000;

/** A request the server refused. `unsaved` is what a refused delete would have lost. */
export class RequestError extends Error {
  constructor(message: string, readonly unsaved?: UnsavedWork[]) {
    super(message);
  }
}

/** Views showing each terminal. */
const channels = new Map<string, Set<TerminalChannel>>();
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
  ws.binaryType = "arraybuffer";
  socket = ws;

  ws.onopen = () => {
    if (socket !== ws) return;
    reconnectDelay = RECONNECT_MIN_MS;
    // The server sends every transcript again.
    clearTranscripts();
    for (const set of channels.values()) for (const channel of set) channel.onOpen();
  };

  ws.onmessage = event => {
    if (socket !== ws) return;
    if (typeof event.data !== "string") {
      const { kind, id, payload } = decodeFrame(event.data);
      for (const channel of channels.get(id) ?? []) {
        if (kind === FRAME_SNAPSHOT) channel.onSnapshot(payload);
        else if (kind === FRAME_OUTPUT) channel.onOutput(payload);
      }
      return;
    }
    const message: ServerMessage = JSON.parse(event.data);
    switch (message.t) {
      case "model":
        applyModel(message.model);
        break;
      case "transcript":
        setTranscript(message.agentId, message.items);
        break;
      case "item":
        upsertItem(message.agentId, message.item);
        break;
      case "info":
        mergeInfo(message.agentId, message.info);
        break;
      case "result": {
        const request = pending.get(message.req);
        pending.delete(message.req);
        if (message.ok) request?.resolve(message.id);
        else request?.reject(new RequestError(message.error, message.unsaved));
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

export function isSocketOpen() {
  return socket?.readyState === WebSocket.OPEN;
}

/** Sends a terminal message if the socket is open. Views resend what they need on reconnect, so drops are fine. */
export function sendTerminal(message: TerminalMessage) {
  if (isSocketOpen()) socket!.send(JSON.stringify(message satisfies ClientMessage));
}

/** Asks the server to change the model. Resolves with the new object's id. */
export function request(message: ModelRequest): Promise<string | undefined> {
  if (!isSocketOpen()) return Promise.reject(new Error("Not connected to the server"));
  const req = nextRequest++;
  return new Promise((resolve, reject) => {
    pending.set(req, { resolve, reject });
    socket!.send(JSON.stringify({ ...message, req } satisfies ClientMessage));
  });
}

export function registerChannel(id: string, channel: TerminalChannel) {
  let set = channels.get(id);
  if (!set) channels.set(id, (set = new Set()));
  set.add(channel);
  if (isSocketOpen()) channel.onOpen();
}

/** Stops showing a terminal. The tab detaches once no view shows it. */
export function unregisterChannel(id: string, channel: TerminalChannel) {
  const set = channels.get(id);
  if (!set?.delete(channel) || set.size > 0) return;
  channels.delete(id);
  sendTerminal({ t: "detach", id });
}
