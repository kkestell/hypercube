import { useSyncExternalStore } from "react";
import { emptyModel, type Model } from "./model";

/** The model as last pushed by the server. Read it from UI with `useModel`. */

let model: Model = emptyModel();
const listeners = new Set<() => void>();

export function applyModel(next: Model) {
  model = next;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** A workspace's sessions and a session's agents, in the order they were created. */
export const sessionsOf = (model: Model, workspaceId: string) => Object.values(model.sessions).filter(session => session.workspaceId === workspaceId);
export const agentsOf = (model: Model, sessionId: string) => Object.values(model.agents).filter(agent => agent.sessionId === sessionId);

export function useModel() {
  return useSyncExternalStore(subscribe, () => model);
}
