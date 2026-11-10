// useReducedMotion follows the reduced-motion setting, for motion that CSS
// does not run.
import {useSyncExternalStore} from 'react';

const reducedMotion = () => matchMedia('(prefers-reduced-motion: reduce)');

function watchReducedMotion(change: () => void) {
  const query = reducedMotion();
  query.addEventListener('change', change);
  return () => query.removeEventListener('change', change);
}

export const useReducedMotion = () => useSyncExternalStore(watchReducedMotion, () => reducedMotion().matches);
