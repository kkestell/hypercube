// The errors the page shows the user: refused actions, failed requests, and
// attachments the composer turns away. Each shows as an error card, newest
// first, until the user closes it.

let count = 0;

export function showError(store, message) {
  const key = `error:${++count}`;
  store.update(state => ({...state, errors: [{key, message}, ...state.errors]}));
}

export const closeError = (state, key) => ({...state, errors: state.errors.filter(error => error.key !== key)});
