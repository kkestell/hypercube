import { useSyncExternalStore } from "react";
import { emptyModel, type Model } from "./model";

/** The model as last pushed by the server. Read it from UI with `useModel`. */

let model: Model = emptyModel();
let received = false;
const listeners = new Set<() => void>();

export function applyModel(next: Model) {
  model = next;
  received = true;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useModel() {
  return useSyncExternalStore(subscribe, () => model);
}

/** Whether the server has sent the model yet. */
export function useModelReceived() {
  return useSyncExternalStore(subscribe, () => received);
}
