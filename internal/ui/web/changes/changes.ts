// The attached editor's Git changes and open diffs, by path relative to the
// target's directory.
import {fetchChanges, HttpError, readDiff} from '../api.ts';
import type {Store} from '../store.ts';
import type {ChangeStatus, ChangeTree, Changes, DiffRead, OpenDiff} from './types.ts';

export const diffReady = (diff: OpenDiff | undefined) => !!diff && (diff.text !== null || diff.message !== null);

const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

const statusLabels: Record<ChangeStatus, string> = {modified: 'Modified', added: 'Added', untracked: 'Untracked', deleted: 'Deleted'};

export const statusLabel = (status: ChangeStatus) => statusLabels[status];

export const statusColor: Record<ChangeStatus, string> = {
  modified: 'text-amber-700 dark:text-amber-500/70', added: 'text-green-700 dark:text-green-500/70', untracked: 'text-green-700 dark:text-green-500/70', deleted: 'text-muted-foreground',
};

export const diffTitle = (path: string, status?: ChangeStatus) =>
  `${path.slice(path.lastIndexOf('/') + 1)} (${status && status !== 'modified' ? statusLabel(status) : 'Working Tree'})`;

// changeTree indexes the changes for the Files tree. A folder takes the status
// its changes share, or modified when they differ. Git names an untracked
// nested repository by its folder, with a trailing slash.
export function changeTree(changes: Changes): ChangeTree {
  const tree: ChangeTree = {files: new Map(), folders: new Map()};
  if (!changes || !('files' in changes)) return tree;
  const mark = (folder: string, status: ChangeStatus) => {
    const current = tree.folders.get(folder);
    tree.folders.set(folder, current && current !== status ? 'modified' : status);
  };
  for (const change of changes.files) {
    const path = change.path.replace(/\/$/, '');
    if (path === change.path) tree.files.set(path, change);
    else mark(path, change.status);
    const parts = path.split('/');
    for (let i = 1; i < parts.length; i++) mark(parts.slice(0, i).join('/'), change.status);
  }
  return tree;
}

export async function loadChanges(store: Store) {
  const editor = store.get().editor;
  if (!editor) return;
  const attachment = store.get().editorVersion;
  const request = store.get().changesVersion + 1;
  store.update(state => ({...state, changesVersion: request}));
  let changes: Changes;
  try {
    const result = await fetchChanges(editor.target);
    changes = result && {...result, files: result.files.toSorted((a, b) => collator.compare(a.path, b.path))};
  } catch (error) {
    changes = error instanceof HttpError && error.status === 404
      ? {missing: true}
      : {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => {
    if (state.editor?.id !== editor.id || state.editorVersion !== attachment || state.changesVersion !== request || JSON.stringify(state.editor.changes) === JSON.stringify(changes)) return state;
    return {...state, editor: {...state.editor, changes}};
  });
}

function openDiff(result: DiffRead): OpenDiff {
  if ('text' in result) return {text: result.text, message: null};
  return {text: null, message: 'message' in result ? result.message : result.error};
}

// Failed reads retain a diff that has already shown text or a message.
export async function readOpenDiff(store: Store, path: string) {
  const editor = store.get().editor;
  if (!editor?.diffs[path]) return;
  const attachment = store.get().editorVersion;
  const request = (store.get().diffReadVersions[path] ?? 0) + 1;
  store.update(state => ({...state, diffReadVersions: {...state.diffReadVersions, [path]: request}}));
  let result: DiffRead;
  try {
    result = await readDiff(editor.target, path);
  } catch (error) {
    result = {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => {
    if (state.editor?.id !== editor.id || state.editorVersion !== attachment || state.diffReadVersions[path] !== request) return state;
    const diff = state.editor.diffs[path];
    if (!diff || ('error' in result && (diff.text !== null || diff.message !== null))) return state;
    const next = openDiff(result);
    if (next.text === diff.text && next.message === diff.message) return state;
    return {...state, editor: {...state.editor, diffs: {...state.editor.diffs, [path]: next}}};
  });
}
