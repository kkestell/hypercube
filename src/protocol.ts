/**
 * Wire protocol for the browser's WebSocket, shared by the server and the
 * browser. Messages are JSON text frames, except terminal bytes, which travel
 * from the server in binary frames: [kind: u8][idLength: u8][id: utf8][payload].
 */

import type { AgentInfo, Model, Session, TranscriptItem, UnsavedWork } from "./model";

/** Scrollback lines kept by the server's headless terminal and by the browser's xterm. */
export const SCROLLBACK = 5000;

/**
 * Terminal messages, by terminal id (a session's id, since a session has one
 * terminal). The browser never creates or closes a terminal; the requests
 * below do.
 */
export type TerminalMessage =
  /** Subscribe to a terminal's output: a snapshot of its screen, then live output. Ignored if there is no such terminal. */
  | { t: "attach"; id: string; cols: number; rows: number }
  | { t: "detach"; id: string }
  | { t: "input"; id: string; data: string }
  | { t: "resize"; id: string; cols: number; rows: number };

/** Requests to change the model. Each gets a `result` with the same `req`. */
export type ModelRequest =
  | { t: "addWorkspace"; repo: string }
  | { t: "createSession"; workspaceId: string; environment: Session["environment"] }
  /** `confirmed` deletes despite unsaved work, after the user was shown it. */
  | { t: "deleteSession"; sessionId: string; confirmed?: boolean }
  | { t: "stopSession"; sessionId: string }
  | { t: "startSession"; sessionId: string }
  | { t: "restartAgent"; agentId: string }
  | { t: "removeWorker"; agentId: string }
  /** Starts the session's shell. Refused unless the session is ready and has none. */
  | { t: "startTerminal"; sessionId: string }
  /** Kills the session's shell and everything it started. */
  | { t: "closeTerminal"; sessionId: string }
  /** A busy agent is steered by default; `queue` waits for the turn to end instead. */
  | { t: "prompt"; agentId: string; text: string; queue?: boolean }
  /** Cancels the turn now instead of after the running tool call, and sends the pending steer. */
  | { t: "interrupt"; agentId: string }
  | { t: "cancel"; agentId: string }
  | { t: "answerPermission"; agentId: string; requestId: string; optionId: string }
  | { t: "setConfigOption"; agentId: string; configId: string; value: string };

export type ClientMessage = TerminalMessage | (ModelRequest & { req: number });

export type ServerMessage =
  /** The whole model. Sent on connect and after every change. */
  | { t: "model"; model: Model }
  /** An agent's whole transcript. Sent on connect, for each agent that has one. */
  | { t: "transcript"; agentId: string; items: TranscriptItem[] }
  /** One item added to or replaced in an agent's transcript. */
  | { t: "item"; agentId: string; item: TranscriptItem }
  /** The fields of an agent's panel info that changed; the browser replaces those. Sent whole on connect. */
  | { t: "info"; agentId: string; info: AgentInfo }
  | { t: "result"; req: number; ok: true; id?: string }
  /** `unsaved` is set when a delete was refused because the session holds work that would be lost. */
  | { t: "result"; req: number; ok: false; error: string; unsaved?: UnsavedWork[] };

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
