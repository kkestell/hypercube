// The attached session's files: the folder listings and expansion of the
// Files tree, and the contents of each file open in a tab, by path relative to
// the session's working directory ('' is the directory itself). Results that
// arrive after the page attached to another session are dropped.
import {HttpError, listFileTree, readFile} from '../api.ts';
import type {ChangeTree} from '../changes/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {FileEntry, FileRead, FolderListing, OpenFile, SessionFiles} from './types.ts';

export const unreadFile: OpenFile = {text: null, message: null, etag: null, deleted: false, source: false, lines: [], reveal: false};

export const fileReady = (file: OpenFile | undefined) => !!file && (file.text !== null || file.message !== null || !!file.preview || file.deleted);

const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

// sortEntries puts folders first, then orders by name as VS Code does.
const sortEntries = (entries: FileEntry[]) => [...entries].sort((a, b) => Number(b.dir) - Number(a.dir) || collator.compare(a.name, b.name));

// listedFolder is whether a folder appears in its parent's listing, rather
// than only through its deleted files.
export function listedFolder(files: SessionFiles, path: string): boolean {
  const slash = path.lastIndexOf('/');
  const name = path.slice(slash + 1);
  return !!files.listings[slash < 0 ? '' : path.slice(0, slash)]?.some(entry => entry.dir && entry.name === name);
}

// treeEntries is a folder's listing with its deleted files added, so the tree
// still shows them and the folders that held them.
export function treeEntries(files: SessionFiles, changes: ChangeTree, dir: string): FileEntry[] {
  const entries = files.listings[dir] ?? [];
  const deleted = new Map<string, FileEntry>();
  for (const [path, change] of changes.files) {
    if (change.status !== 'deleted' || (dir && !path.startsWith(dir + '/'))) continue;
    const rest = dir ? path.slice(dir.length + 1) : path;
    const slash = rest.indexOf('/');
    const name = slash < 0 ? rest : rest.slice(0, slash);
    if (!entries.some(entry => entry.name === name)) deleted.set(name, {name, dir: slash >= 0});
  }
  return deleted.size ? sortEntries([...entries, ...deleted.values()]) : entries;
}

function withFiles(state: State, sessionId: string, change: (files: SessionFiles) => SessionFiles): State {
  if (state.session?.id !== sessionId) return state;
  const files = change(state.session.files);
  return files === state.session.files ? state : {...state, session: {...state.session, files}};
}

// shownFolders includes the root and expanded folders with expanded ancestors.
export function shownFolders(files: SessionFiles): string[] {
  const shown = (dir: string): boolean => dir === '' || (dir in files.expanded && shown(dir.slice(0, Math.max(0, dir.lastIndexOf('/')))));
  return ['', ...Object.keys(files.expanded).filter(shown).sort((a, b) => a.length - b.length)];
}

// fileRead applies a read of an open file. A deleted file keeps its contents,
// and a failed read leaves a file that has shown something as it is.
function fileRead(state: State, sessionId: string, path: string, result: FileRead): State {
  return withFiles(state, sessionId, files => {
    const file = files.open[path];
    if (!file || 'unchanged' in result) return files;
    let next: OpenFile;
    if ('deleted' in result) {
      if (file.deleted) return files;
      next = {...file, deleted: true, etag: null};
    } else if ('error' in result) {
      if (file.text !== null || file.preview || file.message !== null) return files;
      next = {...file, message: result.error};
    } else if ('message' in result) {
      if (result.message === file.message && !file.deleted) return files;
      next = {...file, text: null, preview: undefined, message: result.message, etag: null, deleted: false};
    } else {
      next = {...file, text: result.text, preview: result.preview, message: null, etag: result.etag, deleted: false};
    }
    return {...files, open: {...files.open, [path]: next}};
  });
}

export const setSource = (state: State, path: string, source: boolean): State => state.session ? withFiles(state, state.session.id, files => ({
  ...files, open: {...files.open, [path]: {...files.open[path], source}},
})) : state;

export const revealed = (state: State, sessionId: string, path: string): State => withFiles(state, sessionId, files => ({
  ...files, open: {...files.open, [path]: {...files.open[path], reveal: false}},
}));

// loadTree applies one coherent refresh. A newer read or attachment supersedes
// an older response, including a late response after A → B → A.
export async function loadTree(store: Store, dirs: string[]) {
  const session = store.get().session;
  if (!session) return;
  const attachment = store.get().sessionVersion;
  const request = store.get().treeVersion + 1;
  store.update(state => ({...state, treeVersion: request}));
  const apply = (response: Record<string, FolderListing>) => {
    store.update(state => {
      if (state.session?.id !== session.id || state.sessionVersion !== attachment || state.treeVersion !== request) return state;
      const files = state.session.files;
      const listings = {...files.listings};
      const listingErrors = {...files.listingErrors};
      let changed = false;
      for (const [dir, entries] of Object.entries(response)) {
        if (entries && 'error' in entries) {
          if (listingErrors[dir] !== entries.error) {
            listingErrors[dir] = entries.error;
            changed = true;
          }
          continue;
        }
        if (dir in listingErrors) {
          delete listingErrors[dir];
          changed = true;
        }
        const sorted = entries === null ? null : sortEntries(entries);
        if (JSON.stringify(sorted) !== JSON.stringify(listings[dir])) {
          listings[dir] = sorted;
          changed = true;
        }
      }
      const expanded = {...files.expanded};
      const updated = {...files, listings, listingErrors};
      for (const [dir, entries] of Object.entries(response)) {
        // A deleted-only folder remains navigable through Git changes.
        if (dir && entries === null && expanded[dir] && listedFolder(updated, dir)) {
          delete expanded[dir];
          changed = true;
        }
      }
      return changed ? {...state, session: {...state.session, files: {...updated, expanded}}} : state;
    });
  };
  try {
    apply(await listFileTree(session.id, dirs));
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) apply({'': null});
    else {
      const message = error instanceof Error ? error.message : String(error);
      apply(Object.fromEntries((dirs.includes('') ? [''] : dirs).map(dir => [dir, {error: message}])));
    }
  }
}

// readOpenFile reads an open file, asking only for a change since the last read.
export async function readOpenFile(store: Store, path: string) {
  const session = store.get().session;
  const file = session?.files.open[path];
  if (!session || !file) return;
  const attachment = store.get().sessionVersion;
  const request = (store.get().fileReadVersions[path] ?? 0) + 1;
  store.update(state => ({...state, fileReadVersions: {...state.fileReadVersions, [path]: request}}));
  let result: FileRead;
  try {
    result = await readFile(session.id, path, file.etag);
  } catch (error) {
    result = {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => state.sessionVersion === attachment && state.fileReadVersions[path] === request
    ? fileRead(state, session.id, path, result) : state);
}

// setFolderOpen expands or collapses a folder, listing it when it first opens
// unless it exists only through deleted files.
export function setFolderOpen(store: Store, path: string, open: boolean) {
  const session = store.get().session;
  if (!session || !!session.files.expanded[path] === open) return;
  store.update(state => withFiles(state, session.id, files => {
    const expanded = {...files.expanded};
    if (open) expanded[path] = true;
    else delete expanded[path];
    return {...files, expanded};
  }));
  if (open && !(path in session.files.listings) && listedFolder(session.files, path)) loadTree(store, [path]);
}
