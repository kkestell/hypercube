/**
 * Wire protocol for the single multiplexed terminal WebSocket, shared by the
 * server and the browser.
 *
 * Control messages are JSON text frames. Terminal bytes travel in binary
 * frames: [kind: u8][idLength: u8][id: utf8][payload].
 */

import type { Harness, Model } from "./model";

/** Scrollback lines kept by the server's headless terminal and by every xterm. */
export const SCROLLBACK = 5000;

/** Terminal messages, by terminal id. The browser never creates or closes a terminal; the model does. */
export type TerminalMessage =
  /** Subscribe to a terminal's output. A terminal that doesn't exist gets `gone`. */
  | { t: "attach"; id: string; cols: number; rows: number }
  | { t: "detach"; id: string }
  | { t: "input"; id: string; data: string }
  | { t: "resize"; id: string; cols: number; rows: number };

/** Requests to change the model. Each gets a `result` with the same `req`. */
export type ModelRequest =
  | { t: "addWorkspace"; path: string }
  | { t: "createSession"; workspaceId: string; harness: Harness }
  | { t: "startSession"; sessionId: string }
  | { t: "stopSession"; sessionId: string }
  | { t: "retryAgent"; agentId: string }
  | { t: "restartShell"; workspaceId: string };

export type ClientMessage = TerminalMessage | (ModelRequest & { req: number });

export type TerminalState = "running" | "exited";

export type TerminalStatus = {
  state: TerminalState;
  exitCode: number | null;
  /** Epoch ms of the last output, or null if there has been none. */
  lastOutputAt: number | null;
};

export type ServerMessage =
  /** Sent to every client, throttled. The first one on a connection is `full`. */
  | { t: "status"; full?: true; terminals: Record<string, TerminalStatus>; removed?: string[] }
  /** The terminal's process ended. Sent to attached clients. */
  | { t: "exit"; id: string; exitCode: number | null }
  /** Attach failed: no such terminal. */
  | { t: "gone"; id: string }
  /** The whole model. Sent on connect and after every change. */
  | { t: "model"; model: Model }
  | { t: "result"; req: number; ok: true; id?: string }
  | { t: "result"; req: number; ok: false; error: string };

export const FRAME_SNAPSHOT = 1;
export const FRAME_OUTPUT = 2;
export type FrameKind = typeof FRAME_SNAPSHOT | typeof FRAME_OUTPUT;

const encoder = new TextEncoder();
const decoder = new TextDecoder();

export function encodeFrame(kind: FrameKind, id: string, payload: Uint8Array | string): Uint8Array {
  const idBytes = encoder.encode(id);
  const body = typeof payload === "string" ? encoder.encode(payload) : payload;
  const frame = new Uint8Array(2 + idBytes.length + body.length);
  frame[0] = kind;
  frame[1] = idBytes.length;
  frame.set(idBytes, 2);
  frame.set(body, 2 + idBytes.length);
  return frame;
}

export function decodeFrame(data: ArrayBuffer | Uint8Array): { kind: FrameKind; id: string; payload: Uint8Array } {
  const bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
  const idLength = bytes[1]!;
  return {
    kind: bytes[0] as FrameKind,
    id: decoder.decode(bytes.subarray(2, 2 + idLength)),
    payload: bytes.subarray(2 + idLength),
  };
}
