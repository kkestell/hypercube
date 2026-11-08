// The current editor's tabs, preview, and remembered scroll. File and diff
// tabs share one preview tab, replaced on the next click until kept open.
import {diffReady, diffTitle, readOpenDiff} from '../changes/changes.ts';
import {hasPreview} from '../files/language.ts';
import {fileReady, readOpenFile, unreadFile} from '../files/files.ts';
import type {EditorView} from './types.ts';
import {selectedWorkspace, selectionTarget} from './target.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {ownsScroll} from './view-state.ts';
import * as layout from './tab-layout.ts';
import type {Edge, FileOpenRequest, ScrollMemory, TabEntry, TabId, TabLayout} from './types.ts';

export const emptyTabs: TabLayout = {root: null, preview: null, dragging: null, nextId: 1};

export const dashboardTabId = 'dashboard';
export const dashboardTabs: TabLayout = {...emptyTabs, root: {id: 1, tabs: [dashboardTabId], active: dashboardTabId}, nextId: 2};

export const fileTabId = (path: string): TabId => 'file:' + path;

export const diffTabId = (path: string): TabId => 'diff:' + path;

export const isDiffTab = (id: TabId) => id.startsWith('diff:');

// tabPath is a file or diff tab's path.
export const tabPath = (id: TabId) => id.slice(id.indexOf(':') + 1);

function withEditor(state: State, change: (editor: EditorView) => EditorView): State {
  return state.editor ? {...state, editor: change(state.editor)} : state;
}

const withTabs = (state: State, change: (tabs: TabLayout) => TabLayout) =>
  withEditor(state, editor => ({...editor, tabs: change(editor.tabs)}));

export function tabEntry(state: State, id: TabId): TabEntry {
  if (id === dashboardTabId) {
    const title = selectedWorkspace(state)?.name ?? 'Workspace';
    return {kind: 'dashboard', id, label: title, title, preview: false};
  }
  const {editor} = state;
  if (!editor) throw new Error(`Tab ${id} has no attached editor.`);
  const path = tabPath(id);
  const name = path.slice(path.lastIndexOf('/') + 1);
  const preview = editor.tabs.preview === id;
  if (isDiffTab(id)) {
    const changes = editor.changes;
    const status = changes && 'files' in changes ? changes.files.find(file => file.path === path)?.status : undefined;
    const label = diffTitle(path, status);
    return {kind: 'diff', id, path, name, label, title: label, preview};
  }
  const open = editor.files.open[path];
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
function forgetTab(editor: EditorView, id: TabId): EditorView {
  const path = tabPath(id);
  const scroll = {...editor.scroll};
  delete scroll[id];
  if (isDiffTab(id)) {
    const diffs = {...editor.diffs};
    delete diffs[path];
    return {...editor, diffs, scroll};
  }
  const open = {...editor.files.open};
  delete open[path];
  return {...editor, files: {...editor.files, open}, scroll};
}

export function closeTab(state: State, id: TabId): State {
  return withEditor(state, editor => {
    const preview = editor.tabs.preview === id ? null : editor.tabs.preview;
    return {...forgetTab(editor, id), tabs: {...layout.closeTab(editor.tabs, id), preview}};
  });
}

// tabOpened opens a tab as the shared preview, replacing the previous preview,
// and has add record the new tab's data. An open tab is only selected.
function tabOpened(state: State, id: TabId, add: (editor: EditorView) => EditorView): State {
  return withEditor(state, editor => {
    if (layout.findPane(editor.tabs.root, id)) return {...editor, tabs: layout.openTab(editor.tabs, id)};
    const preview = editor.tabs.preview;
    const tabs = preview ? layout.replaceTab(editor.tabs, preview, id) : layout.openTab(editor.tabs, id);
    const kept = preview ? forgetTab(editor, preview) : editor;
    return {...add(kept), tabs: {...tabs, preview: id}};
  });
}

// File and diff clicks open the shared preview, or select an existing tab.
export function openFileTab(store: Store, path: string) {
  if (!store.get().editor) return;
  const ready = fileReady(store.get().editor?.files.open[path]);
  store.update(state => tabOpened(state, fileTabId(path), editor => ({
    ...editor, files: {...editor.files, open: {...editor.files.open, [path]: unreadFile}},
  })));
  if (ready) readOpenFile(store, path);
}

export function openDiffTab(store: Store, path: string) {
  if (!store.get().editor) return;
  const ready = diffReady(store.get().editor?.diffs[path]);
  store.update(state => tabOpened(state, diffTabId(path), editor => ({
    ...editor, diffs: {...editor.diffs, [path]: {text: null, message: null}},
  })));
  if (ready) readOpenDiff(store, path);
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
  openFileAt(store, path, lines);
}

// Search navigation opens in the current editor, independent of agent requests.
export function openFileAt(store: Store, path: string, lines: FileOpenRequest['lines']) {
  if (!store.get().editor) return;
  const ready = fileReady(store.get().editor?.files.open[path]);
  store.update(state => withEditor(state, editor => {
    const id = fileTabId(path);
    const tabs = layout.openTab(editor.tabs, id);
    const file = {...(editor.files.open[path] ?? unreadFile), source: hasPreview(path) && lines.length > 0, lines, reveal: lines.length > 0};
    return {...editor, tabs: {...tabs, preview: tabs.preview === id ? null : tabs.preview}, files: {...editor.files, open: {...editor.files.open, [path]: file}}};
  }));
  if (ready) readOpenFile(store, path);
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

export const keepOpen = (state: State, id: TabId) =>
  withTabs(state, tabs => tabs.preview === id ? {...tabs, preview: null} : tabs);

export const selectTab = (state: State, paneId: number, id: TabId) =>
  withTabs(state, tabs => layout.selectTab(tabs, paneId, id));

export const moveTab = (state: State, id: TabId, paneId: number, edge?: Edge, before?: TabId | null) =>
  withTabs(state, tabs => ({...layout.moveTab(tabs, id, paneId, edge, before), dragging: null}));

export const setDragging = (state: State, id: TabId | null) => withTabs(state, tabs => ({...tabs, dragging: id}));

export const resizeSplit = (state: State, splitId: number, ratio: number) =>
  withTabs(state, tabs => layout.resizeSplit(tabs, splitId, Math.max(15, Math.min(85, ratio))));

export const activatePane = (state: State, id: number | null): State =>
  state.editor && state.editor.activePane !== id ? withEditor(state, editor => ({...editor, activePane: id})) : state;

// rememberScroll also accepts a departing view's final scroll after selection.
export function rememberScroll(state: State, sessionId: string, id: TabId, scroll: ScrollMemory): State {
  const active = state.editor?.id === sessionId;
  const target = selectionTarget(sessionId);
  const workspace = target && 'workspace' in target ? target.workspace : null;
  const view = active ? state.editor : workspace ? state.workspaceViews[workspace] : state.sessionViews[sessionId];
  if (!view || !ownsScroll(view, sessionId, id)) return state;
  const old = view.scroll[id];
  if (old?.top === scroll.top && old.left === scroll.left && old.following === scroll.following) return state;
  const remembered = {...view.scroll, [id]: scroll};
  if (active && state.editor) return {...state, editor: {...state.editor, scroll: remembered}};
  return workspace ? {...state, workspaceViews: {...state.workspaceViews, [workspace]: {...state.workspaceViews[workspace], scroll: remembered}}}
    : {...state, sessionViews: {...state.sessionViews, [sessionId]: {...state.sessionViews[sessionId], scroll: remembered}}};
}
