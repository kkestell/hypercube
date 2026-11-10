// The errors the page shows the user: refused actions, failed requests, and
// attachments the composer turns away. Each shows as an error card, newest
// first, until the user closes it.

import type {Store} from '../store.ts';
import type {State} from '../state.ts';

export function showError(store: Store, message: string) {
  const key = `error:${crypto.randomUUID()}`;
  store.update(state => ({...state, errors: [{key, message}, ...state.errors]}));
}

export const closeError = (state: State, key: string): State => ({...state, errors: state.errors.filter(error => error.key !== key)});
