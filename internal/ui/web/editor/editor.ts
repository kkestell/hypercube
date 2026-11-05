// The attached session's tabs, preview, and remembered scroll. File and diff
// tabs share one preview tab, replaced on the next click until kept open.
import {diffTitle, readOpenDiff} from '../changes/changes.ts';
import type {OpenFile} from '../files/types.ts';
import {hasPreview} from '../files/language.ts';
import {readOpenFile} from '../files/files.ts';
import {shownSession} from '../sessions/selection.ts';
import type {AttachedSession} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import * as layout from './tab-layout.ts';
import type {Edge, FileOpenRequest, ScrollMemory, TabEntry, TabId, TabLayout} from './types.ts';

export const agentTabId = 'agent';

// agentTabs is the layout a session starts with: its agent alone.
export const agentTabs: TabLayout = {root: {id: 1, tabs: [agentTabId], active: agentTabId}, preview: null, dragging: null, nextId: 2};

export const browserTabId = 'browser';

export const fileTabId = (path: string): TabId => 'file:' + path;

export const diffTabId = (path: string): TabId => 'diff:' + path;

export const isDiffTab = (id: TabId) => id.startsWith('diff:');

// tabPath is a file or diff tab's path.
export const tabPath = (id: TabId) => id.slice(id.indexOf(':') + 1);

function withSession(state: State, change: (session: AttachedSession) => AttachedSession): State {
  return state.session ? {...state, session: change(state.session)} : state;
}

const withTabs = (state: State, change: (tabs: TabLayout) => TabLayout) =>
  withSession(state, session => ({...session, tabs: change(session.tabs)}));

export function tabEntry(state: State, id: TabId): TabEntry {
  if (id === agentTabId) {
    const title = shownSession(state)?.title ?? 'New Session';
    return {kind: 'agent', id, label: title, title, preview: false};
  }
  const {session} = state;
  if (!session) throw new Error(`Tab ${id} has no attached session.`);
  if (id === browserTabId) {
    const page = state.browsers[session.id];
    const label = page?.title || 'Browser';
    return {kind: 'browser', id, label, title: page?.url || label, preview: false};
  }
  const path = tabPath(id);
  const name = path.slice(path.lastIndexOf('/') + 1);
  const preview = session.tabs.preview === id;
  if (isDiffTab(id)) {
    const changes = session.changes;
    const status = changes && 'files' in changes ? changes.files.find(file => file.path === path)?.status : undefined;
    const label = diffTitle(path, status);
    return {kind: 'diff', id, path, name, label, title: label, preview};
  }
  const open = session.files.open[path];
  const deleted = !!open?.deleted;
  return {
    kind: 'file', id, path, name, deleted,
    source: hasPreview(path) && (!open?.preview || open.text !== null) ? open?.source ?? false : null,
    label: name,
    title: path + (deleted ? ' (deleted)' : ''),
    preview,
  };
}

// forgetTab drops a file or diff's data and scroll when closed or replaced.
function forgetTab(session: AttachedSession, id: TabId): AttachedSession {
  const path = tabPath(id);
  const scroll = {...session.scroll};
  delete scroll[id];
  if (isDiffTab(id)) {
    const diffs = {...session.diffs};
    delete diffs[path];
    return {...session, diffs, scroll};
  }
  const open = {...session.files.open};
  delete open[path];
  return {...session, files: {...session.files, open}, scroll};
}

export function closeTab(state: State, id: TabId): State {
  if (id === browserTabId) return withTabs(state, tabs => layout.closeTab(tabs, id));
  return withSession(state, session => {
    const preview = session.tabs.preview === id ? null : session.tabs.preview;
    return {...forgetTab(session, id), tabs: {...layout.closeTab(session.tabs, id), preview}};
  });
}

const unread: OpenFile = {text: null, message: null, etag: null, deleted: false, source: false, lines: [], reveal: false};

// tabOpened opens a tab as the shared preview, replacing the previous preview,
// and has add record the new tab's data. An open tab is only selected.
function tabOpened(state: State, id: TabId, add: (session: AttachedSession) => AttachedSession): State {
  return withSession(state, session => {
    if (layout.findPane(session.tabs.root, id)) return {...session, tabs: layout.openTab(session.tabs, id)};
    const preview = session.tabs.preview;
    const tabs = preview ? layout.replaceTab(session.tabs, preview, id) : layout.openTab(session.tabs, id);
    const kept = preview ? forgetTab(session, preview) : session;
    return {...add(kept), tabs: {...tabs, preview: id}};
  });
}

// File and diff clicks open the shared preview, or select an existing tab.
export function openFileTab(store: Store, path: string) {
  if (!store.get().session) return;
  store.update(state => tabOpened(state, fileTabId(path), session => ({
    ...session, files: {...session.files, open: {...session.files.open, [path]: unread}},
  })));
  readOpenFile(store, path);
}

export function openDiffTab(store: Store, path: string) {
  if (!store.get().session) return;
  store.update(state => tabOpened(state, diffTabId(path), session => ({
    ...session, diffs: {...session.diffs, [path]: {text: null, message: null}},
  })));
  readOpenDiff(store, path);
}

// openRequestedFile keeps and selects an agent's file. A late socket message
// waits by session until the page attaches to it again.
export function openRequestedFile(store: Store, request: FileOpenRequest) {
  const {session: sessionId, path, lines} = request;
  if (store.get().session?.id !== sessionId) {
    store.update(state => ({...state, fileOpens: {
      ...state.fileOpens, [sessionId]: [...(state.fileOpens[sessionId] ?? []), request],
    }}));
    return;
  }
  store.update(state => withSession(state, session => {
    const id = fileTabId(path);
    const tabs = layout.openTab(session.tabs, id);
    const file = {...(session.files.open[path] ?? unread), source: hasPreview(path) && lines.length > 0, lines, reveal: lines.length > 0};
    return {...session, tabs: {...tabs, preview: tabs.preview === id ? null : tabs.preview}, files: {...session.files, open: {...session.files.open, [path]: file}}};
  }));
  readOpenFile(store, path);
}

// openPendingFiles runs when the attached session changes.
export function openPendingFiles(store: Store) {
  const id = store.get().session?.id;
  if (id === undefined) return;
  const requests = store.get().fileOpens[id];
  if (!requests) return;
  store.update(state => {
    const fileOpens = {...state.fileOpens};
    delete fileOpens[id];
    return {...state, fileOpens};
  });
  for (const request of requests) openRequestedFile(store, request);
}

// openBrowserTab opens the browser's tab, as a file opens, and selects it if
// select is set or it was closed. A shown chat stays shown, keeping the
// composer and its focus, with the browser's tab behind it.
export const openBrowserTab = (state: State, select: boolean) => withTabs(state, tabs => {
  if (layout.findPane(tabs.root, browserTabId) && !select) return tabs;
  const next = layout.openTab(tabs, browserTabId);
  const pane = layout.findPane(next.root, browserTabId);
  const chatShown = pane && layout.panes(tabs.root).some(p => p.id === pane.id && p.active === agentTabId);
  return chatShown ? layout.selectTab(next, pane.id, agentTabId) : next;
});

export const keepOpen = (state: State, id: TabId) =>
  withTabs(state, tabs => tabs.preview === id ? {...tabs, preview: null} : tabs);

export const selectTab = (state: State, paneId: number, id: TabId) =>
  withTabs(state, tabs => layout.selectTab(tabs, paneId, id));

export const moveTab = (state: State, id: TabId, paneId: number, edge?: Edge, before?: TabId | null) =>
  withTabs(state, tabs => ({...layout.moveTab(tabs, id, paneId, edge, before), dragging: null}));

export const setDragging = (state: State, id: TabId | null) => withTabs(state, tabs => ({...tabs, dragging: id}));

export const resizeSplit = (state: State, splitId: number, ratio: number) =>
  withTabs(state, tabs => layout.resizeSplit(tabs, splitId, Math.max(15, Math.min(85, ratio))));

// rememberScroll keeps a view's scroll while its session stays attached, for
// agents and for files that are still open. A closed tab's view unmounts
// after the tab is forgotten.
export function rememberScroll(state: State, sessionId: string, id: TabId, scroll: ScrollMemory): State {
  if (!state.session || state.session.id !== sessionId) return state;
  const kept = layout.findPane(state.session.tabs.root, id) || shownSession(state)?.id === id;
  if (!kept) return state;
  return withSession(state, session => ({...session, scroll: {...session.scroll, [id]: scroll}}));
}
