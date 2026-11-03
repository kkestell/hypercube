// The tabbed area of the attached session: its tabs, the preview tab, and each
// view's remembered scroll. One tab is the session's agent, which never
// closes; one can be its browser; every other tab is a file, by fileTabId.
// As in VS Code, a file
// opened with one click shows in the preview tab, which the next file opened
// replaces until it is kept open.
import {hasPreview} from '../files/language.js';
import {readOpenFile} from '../files/files.js';
import {shownSession} from '../sessions/selection.js';
import * as layout from './tab-layout.js';

export const agentTabId = 'agent';

// agentTabs is the layout a session starts with: its agent alone.
export const agentTabs = {root: {id: 1, tabs: [agentTabId], active: agentTabId}, preview: null, dragging: null, nextId: 2};

export const browserTabId = 'browser';

export const fileTabId = path => 'file:' + path;

// tabPath is a file tab's path.
export const tabPath = id => id.slice('file:'.length);

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
  const deleted = !!session.files.open[path]?.deleted;
  return {
    id, path, deleted,
    source: hasPreview(path) ? session.files.open[path]?.source ?? false : null,
    label: path.slice(path.lastIndexOf('/') + 1),
    title: path + (deleted ? ' (deleted)' : ''),
    preview: session.tabs.preview === id,
  };
}

// closeTab closes a file and forgets its scroll, or closes the browser's tab.
export function closeTab(state, id) {
  if (id === browserTabId) return withTabs(state, tabs => layout.closeTab(tabs, id));
  return withSession(state, session => {
    const path = tabPath(id);
    const open = {...session.files.open};
    delete open[path];
    const scroll = {...session.scroll};
    delete scroll[id];
    const preview = session.tabs.preview === id ? null : session.tabs.preview;
    return {...session, tabs: {...layout.closeTab(session.tabs, id), preview}, files: {...session.files, open}, scroll};
  });
}

const unread = {text: null, message: null, etag: null, deleted: false, source: false, lines: [], reveal: false};

function fileTabOpened(state, path) {
  const id = fileTabId(path);
  return withSession(state, session => {
    if (layout.findPane(session.tabs.root, id)) return {...session, tabs: layout.openTab(session.tabs, id)};
    const preview = session.tabs.preview;
    const tabs = preview ? layout.replaceTab(session.tabs, preview, id) : layout.openBeside(session.tabs, id, agentTabId);
    const open = {...session.files.open, [path]: unread};
    const scroll = {...session.scroll};
    if (preview) {
      delete open[tabPath(preview)];
      delete scroll[preview];
    }
    return {...session, tabs: {...tabs, preview: id}, files: {...session.files, open}, scroll};
  });
}

// openFileTab shows a file in the preview tab, or selects its open tab, and
// reads it.
export function openFileTab(store, path) {
  if (!store.get().session) return;
  store.update(state => fileTabOpened(state, path));
  readOpenFile(store, path);
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
