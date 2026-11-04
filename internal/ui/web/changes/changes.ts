// The attached session's repository changes and open diffs. Paths are relative
// to the repository root, independently of file tabs' session-relative paths.
import {fetchChanges, HttpError, readDiff} from '../api.ts';
import type {Store} from '../store.ts';
import type {ChangeStatus, Changes, DiffRead, OpenDiff} from './types.ts';

const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

const statusLabels: Record<ChangeStatus, string> = {modified: 'Modified', added: 'Added', untracked: 'Untracked', deleted: 'Deleted'};

export const statusLabel = (status: ChangeStatus) => statusLabels[status];

export const diffTitle = (path: string, status?: ChangeStatus) =>
  `${path.slice(path.lastIndexOf('/') + 1)} (${status && status !== 'modified' ? statusLabel(status) : 'Working Tree'})`;

export async function loadChanges(store: Store) {
  const session = store.get().session;
  if (!session) return;
  let changes: Changes;
  try {
    const result = await fetchChanges(session.id);
    changes = result && {...result, files: result.files.toSorted((a, b) => collator.compare(a.path, b.path))};
  } catch (error) {
    changes = error instanceof HttpError && error.status === 404
      ? {missing: true}
      : {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => {
    if (state.session?.id !== session.id || JSON.stringify(state.session.changes) === JSON.stringify(changes)) return state;
    return {...state, session: {...state.session, changes}};
  });
}

function openDiff(result: DiffRead): OpenDiff {
  if ('text' in result) return {text: result.text, message: null};
  return {text: null, message: 'message' in result ? result.message : result.error};
}

// Failed reads retain a diff that has already shown text or a message.
export async function readOpenDiff(store: Store, path: string) {
  const session = store.get().session;
  if (!session?.diffs[path]) return;
  let result: DiffRead;
  try {
    result = await readDiff(session.id, path);
  } catch (error) {
    result = {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => {
    if (state.session?.id !== session.id) return state;
    const diff = state.session.diffs[path];
    if (!diff || ('error' in result && (diff.text !== null || diff.message !== null))) return state;
    const next = openDiff(result);
    if (next.text === diff.text && next.message === diff.message) return state;
    return {...state, session: {...state.session, diffs: {...state.session.diffs, [path]: next}}};
  });
}
