// The attached session's files: the folder listings and expansion of the
// Files tree, and the contents of each file open in a tab, by path relative to
// the session's working directory ('' is the directory itself). Results that
// arrive after the page attached to another session are dropped.
import {HttpError, listFiles, readFile} from '../api.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {FileEntry, FileRead, OpenFile, SessionFiles} from './types.ts';

const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

// sortEntries puts folders first, then orders by name as VS Code does.
const sortEntries = (entries: FileEntry[]) => [...entries].sort((a, b) => Number(b.dir) - Number(a.dir) || collator.compare(a.name, b.name));

function withFiles(state: State, sessionId: string, change: (files: SessionFiles) => SessionFiles): State {
  if (state.session?.id !== sessionId) return state;
  const files = change(state.session.files);
  return files === state.session.files ? state : {...state, session: {...state.session, files}};
}

function listed(state: State, sessionId: string, dir: string, entries: FileEntry[]): State {
  return withFiles(state, sessionId, files => {
    const sorted = sortEntries(entries);
    if (JSON.stringify(sorted) === JSON.stringify(files.listings[dir])) return files;
    return {...files, listings: {...files.listings, [dir]: sorted}};
  });
}

// listingMissing records a missing folder and forgets its expansion.
function listingMissing(state: State, sessionId: string, dir: string): State {
  return withFiles(state, sessionId, files => {
    if (files.listings[dir] === null && !files.expanded[dir]) return files;
    const listings = {...files.listings}, expanded = {...files.expanded};
    listings[dir] = null;
    delete expanded[dir];
    return {...files, listings, expanded};
  });
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
      if (file.text !== null || file.message !== null) return files;
      next = {...file, message: result.error};
    } else if ('message' in result) {
      if (result.message === file.message && !file.deleted) return files;
      next = {...file, text: null, message: result.message, etag: null, deleted: false};
    } else {
      next = {...file, text: result.text, message: null, etag: result.etag, deleted: false};
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

// loadFolder lists a folder. An unreadable folder shows no children.
export async function loadFolder(store: Store, sessionId: string, dir: string) {
  try {
    const entries = await listFiles(sessionId, dir);
    store.update(state => listed(state, sessionId, dir, entries));
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) store.update(state => listingMissing(state, sessionId, dir));
  }
}

// readOpenFile reads an open file, asking only for a change since the last read.
export async function readOpenFile(store: Store, path: string) {
  const session = store.get().session;
  const file = session?.files.open[path];
  if (!session || !file) return;
  let result: FileRead;
  try {
    result = await readFile(session.id, path, file.etag);
  } catch (error) {
    result = {error: error instanceof Error ? error.message : String(error)};
  }
  store.update(state => fileRead(state, session.id, path, result));
}

// setFolderOpen expands or collapses a folder, listing it when it first opens.
export function setFolderOpen(store: Store, path: string, open: boolean) {
  const session = store.get().session;
  if (!session || !!session.files.expanded[path] === open) return;
  store.update(state => withFiles(state, session.id, files => {
    const expanded = {...files.expanded};
    if (open) expanded[path] = true;
    else delete expanded[path];
    return {...files, expanded};
  }));
  if (open && !(path in session.files.listings)) loadFolder(store, session.id, path);
}
