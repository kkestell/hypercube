// Keep the native Dock and menu bar counts in step with waiting sessions.
import {waitingCount} from '../sessions/selection.js';

export function startBadge(store) {
  const handler = globalThis.webkit?.messageHandlers?.window;
  if (!handler) return;
  let count;
  const update = state => {
    const next = waitingCount(state);
    if (next === count) return;
    count = next;
    handler.postMessage({type: 'badge', count});
  };
  store.subscribe(update);
  update(store.get());
}
