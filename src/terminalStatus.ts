import { useSyncExternalStore } from "react";
import type { ServerMessage, TerminalStatus } from "./protocol";

/**
 * What the server says about every terminal it runs, including ones this tab
 * has not opened: running or exited, and when each last produced output.
 * Updated from the status stream; read it from UI with the hooks below.
 */

type Snapshot = {
  /** The terminal socket is currently open. Statuses are stale while it is not. */
  connected: boolean;
  terminals: Readonly<Record<string, TerminalStatus>>;
};

let snapshot: Snapshot = { connected: false, terminals: {} };
const listeners = new Set<() => void>();

function update(next: Snapshot) {
  snapshot = next;
  for (const listener of listeners) listener();
}

export function setTerminalSocketConnected(connected: boolean) {
  if (snapshot.connected !== connected) update({ ...snapshot, connected });
}

export function applyStatusMessage(message: Extract<ServerMessage, { t: "status" }>) {
  const terminals = message.full ? { ...message.terminals } : { ...snapshot.terminals, ...message.terminals };
  for (const id of message.removed ?? []) delete terminals[id];
  update({ ...snapshot, terminals });
}

export function getTerminalStatuses() {
  return snapshot.terminals;
}

export function getTerminalStatus(id: string): TerminalStatus | undefined {
  return snapshot.terminals[id];
}

export function subscribeTerminalStatuses(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

const subscribe = subscribeTerminalStatuses;

export function useTerminalStatuses() {
  return useSyncExternalStore(subscribe, () => snapshot.terminals);
}

export function useTerminalStatus(id: string) {
  return useSyncExternalStore(subscribe, () => snapshot.terminals[id]);
}

export function useTerminalSocketConnected() {
  return useSyncExternalStore(subscribe, () => snapshot.connected);
}
