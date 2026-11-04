// Keep the native Dock and menu bar counts in step with waiting sessions.
import {waitingCount} from '../sessions/selection.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {windowHandler} from './window.ts';

export function startBadge(store: Store) {
  const handler = windowHandler();
  if (!handler) return;
  let count: number | undefined;
  const update = (state: State) => {
    if (!state.loaded) return;
    const next = waitingCount(state);
    if (next === count) return;
    count = next;
    handler.postMessage({type: 'badge', count});
  };
  store.subscribe(update);
}
