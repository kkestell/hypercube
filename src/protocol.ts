/**
 * Wire protocol for the browser's WebSocket, shared by the server and the
 * browser. Every message is a JSON text frame.
 */

import type { Model } from "./model";

/** Requests to change the model. Each gets a `result` with the same `req`. */
export type ModelRequest =
  | { t: "addWorkspace"; path: string }
  | { t: "createSession"; workspaceId: string };

export type ClientMessage = ModelRequest & { req: number };

export type ServerMessage =
  /** The whole model. Sent on connect and after every change. */
  | { t: "model"; model: Model }
  | { t: "result"; req: number; ok: true; id?: string }
  | { t: "result"; req: number; ok: false; error: string };
