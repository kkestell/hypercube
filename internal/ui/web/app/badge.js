// Keep the native Dock and menu bar counts in step with waiting sessions.
import {waitingCount} from '../sessions/selection.js';
import {windowHandler} from './window.js';

export function startBadge(store) {
  const handler = windowHandler();
  if (!handler) return;
  let count;
  const update = state => {
    if (!state.loaded) return;
    const next = waitingCount(state);
    if (next === count) return;
    count = next;
    handler.postMessage({type: 'badge', count});
  };
  store.subscribe(update);
}
