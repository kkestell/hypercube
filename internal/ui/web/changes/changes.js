// The attached session's repository changes and open diffs. Paths are relative
// to the repository root, independently of file tabs' session-relative paths.
import {fetchChanges, readDiff} from '../api.js';

const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

export const statusLabel = status => ({modified: 'Modified', added: 'Added', untracked: 'Untracked', deleted: 'Deleted'})[status];

export const diffTitle = (path, status) => `${path.slice(path.lastIndexOf('/') + 1)} (${status && status !== 'modified' ? statusLabel(status) : 'Working Tree'})`;

export async function loadChanges(store) {
  const session = store.get().session;
  if (!session) return;
  let changes;
  try {
    changes = await fetchChanges(session.id);
    if (changes) changes = {...changes, files: [...changes.files].sort((a, b) => collator.compare(a.path, b.path))};
  } catch (error) {
    changes = error.status === 404 ? {missing: true} : {error: error.message};
  }
  store.update(state => {
    if (state.session?.id !== session.id || JSON.stringify(state.session.changes) === JSON.stringify(changes)) return state;
    return {...state, session: {...state.session, changes}};
  });
}

// Failed reads retain a diff that has already shown text or a message.
export async function readOpenDiff(store, path) {
  const session = store.get().session;
  if (!session?.diffs[path]) return;
  let result;
  try {
    result = await readDiff(session.id, path);
  } catch (error) {
    result = {error: error.message};
  }
  store.update(state => {
    if (state.session?.id !== session.id) return state;
    const diff = state.session.diffs[path];
    if (!diff || (result.error && (diff.text !== null || diff.message !== null))) return state;
    const next = {text: result.text ?? null, message: result.message ?? result.error ?? null};
    if (next.text === diff.text && next.message === diff.message) return state;
    return {...state, session: {...state.session, diffs: {...state.session.diffs, [path]: next}}};
  });
}
