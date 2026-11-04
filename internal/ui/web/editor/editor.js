// The attached session's tabs, preview, and remembered scroll. File and diff
// tabs share one preview tab, replaced on the next click until kept open.
import {diffTitle, readOpenDiff} from '../changes/changes.js';
import {hasPreview} from '../files/language.js';
import {readOpenFile} from '../files/files.js';
import {shownSession} from '../sessions/selection.js';
import * as layout from './tab-layout.js';

export const agentTabId = 'agent';

// agentTabs is the layout a session starts with: its agent alone.
export const agentTabs = {root: {id: 1, tabs: [agentTabId], active: agentTabId}, preview: null, dragging: null, nextId: 2};

export const browserTabId = 'browser';

export const fileTabId = path => 'file:' + path;

export const diffTabId = path => 'diff:' + path;

export const isDiffTab = id => id.startsWith('diff:');

// tabPath is a file or diff tab's path.
export const tabPath = id => id.slice(id.indexOf(':') + 1);

function withSession(state, change) {
  return state.session ? {...state, session: change(state.session)} : state;
}

const withTabs = (state, change) => withSession(state, session => ({...session, tabs: change(session.tabs)}));

// tabEntry describes a tab for its tab strip.
export function tabEntry(state, id) {
  if (id === agentTabId) {
    const title = shownSession(state)?.title ?? 'New Session';
    return {id, agent: true, label: title, title, preview: false, deleted: false};
  }
  const {session} = state;
  if (id === browserTabId) {
    const page = state.browsers[session.id];
    const label = page?.title || 'Browser';
    return {id, browser: true, label, title: page?.url || label, preview: false, deleted: false};
  }
  const path = tabPath(id);
  const name = path.slice(path.lastIndexOf('/') + 1);
  if (isDiffTab(id)) {
    const status = session.changes?.files?.find(file => file.path === path)?.status;
    const label = diffTitle(path, status);
    return {id, path, name, diff: true, label, title: label, preview: session.tabs.preview === id};
  }
  const deleted = !!session.files.open[path]?.deleted;
  return {
    id, path, name, deleted,
    source: hasPreview(path) ? session.files.open[path]?.source ?? false : null,
    label: name,
    title: path + (deleted ? ' (deleted)' : ''),
    preview: session.tabs.preview === id,
  };
}

// forgetTab drops a file or diff's data and scroll when closed or replaced.
function forgetTab(session, id) {
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

export function closeTab(state, id) {
  if (id === browserTabId) return withTabs(state, tabs => layout.closeTab(tabs, id));
  return withSession(state, session => {
    const preview = session.tabs.preview === id ? null : session.tabs.preview;
    return {...forgetTab(session, id), tabs: {...layout.closeTab(session.tabs, id), preview}};
  });
}

const unread = {text: null, message: null, etag: null, deleted: false, source: false, lines: [], reveal: false};

function tabOpened(state, id, data) {
  return withSession(state, session => {
    if (layout.findPane(session.tabs.root, id)) return {...session, tabs: layout.openTab(session.tabs, id)};
    const preview = session.tabs.preview;
    const tabs = preview ? layout.replaceTab(session.tabs, preview, id) : layout.openBeside(session.tabs, id, agentTabId);
    const kept = preview ? forgetTab(session, preview) : session;
    const path = tabPath(id);
    const contents = isDiffTab(id)
      ? {diffs: {...kept.diffs, [path]: data}}
      : {files: {...kept.files, open: {...kept.files.open, [path]: data}}};
    return {...kept, ...contents, tabs: {...tabs, preview: id}};
  });
}

// File and diff clicks open the shared preview, or select an existing tab.
export function openFileTab(store, path) {
  if (!store.get().session) return;
  store.update(state => tabOpened(state, fileTabId(path), unread));
  readOpenFile(store, path);
}

export function openDiffTab(store, path) {
  if (!store.get().session) return;
  store.update(state => tabOpened(state, diffTabId(path), {text: null, message: null}));
  readOpenDiff(store, path);
}

// openRequestedFile keeps and selects an agent's file. A late socket message
// waits by session until the page attaches to it again.
export function openRequestedFile(store, request) {
  const {session: sessionId, path, lines} = request;
  if (store.get().session?.id !== sessionId) {
    store.update(state => ({...state, fileOpens: {
      ...state.fileOpens, [sessionId]: [...(state.fileOpens?.[sessionId] ?? []), request],
    }}));
    return;
  }
  store.update(state => withSession(state, session => {
    const id = fileTabId(path);
    const tabs = layout.openBeside(session.tabs, id, agentTabId);
    const file = {...(session.files.open[path] ?? unread), source: hasPreview(path) && lines.length > 0, lines, reveal: lines.length > 0};
    return {...session, tabs: {...tabs, preview: tabs.preview === id ? null : tabs.preview}, files: {...session.files, open: {...session.files.open, [path]: file}}};
  }));
  readOpenFile(store, path);
}

// openPendingFiles runs when the attached session changes.
export function openPendingFiles(store) {
  const id = store.get().session?.id;
  const requests = store.get().fileOpens?.[id];
  if (!requests) return;
  store.update(state => {
    const fileOpens = {...state.fileOpens};
    delete fileOpens[id];
    return {...state, fileOpens};
  });
  for (const request of requests) openRequestedFile(store, request);
}

// openBrowserTab opens the browser's tab beside the agent, as a file opens,
// and selects it if select is set or it was closed.
export const openBrowserTab = (state, select) => withTabs(state, tabs => {
  if (!layout.findPane(tabs.root, browserTabId)) return layout.openBeside(tabs, browserTabId, agentTabId);
  return select ? layout.openTab(tabs, browserTabId) : tabs;
});

export const keepOpen = (state, id) => withTabs(state, tabs => tabs.preview === id ? {...tabs, preview: null} : tabs);

export const selectTab = (state, paneId, id) => withTabs(state, tabs => layout.selectTab(tabs, paneId, id));

export const moveTab = (state, id, paneId, edge, before) =>
  withTabs(state, tabs => ({...layout.moveTab(tabs, id, paneId, edge, before), dragging: null}));

export const setDragging = (state, id) => withTabs(state, tabs => ({...tabs, dragging: id}));

export const resizeSplit = (state, splitId, ratio) =>
  withTabs(state, tabs => layout.resizeSplit(tabs, splitId, Math.max(15, Math.min(85, ratio))));

// rememberScroll keeps a view's scroll while its session stays attached, for
// agents and for files that are still open. A closed tab's view unmounts
// after the tab is forgotten.
export function rememberScroll(state, sessionId, id, scroll) {
  if (!state.session || state.session.id !== sessionId) return state;
  const kept = layout.findPane(state.session.tabs.root, id) || shownSession(state)?.id === id;
  if (!kept) return state;
  return withSession(state, session => ({...session, scroll: {...session.scroll, [id]: scroll}}));
}
