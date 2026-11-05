// Keep the desktop app's Dock and menu bar counts in step with waiting sessions.
import {waitingCount} from '../sessions/selection.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {hypercube} from './window.ts';

export function startBadge(store: Store) {
  const bridge = hypercube();
  if (!bridge) return;
  let count: number | undefined;
  const update = (state: State) => {
    if (!state.loaded) return;
    const next = waitingCount(state);
    if (next === count) return;
    count = next;
    bridge.postMessage({type: 'badge', count});
  };
  store.subscribe(update);
}
