// Attaching a view parks only user choices and cached folder listings. Reads
// and transcripts refresh, and request generations prevent late results crossing views.
import {agentTabs, dashboardTabs} from './editor.ts';
import {selectionTarget, targetExists, targetKey, type EditorTarget} from './target.ts';
import type {EditorView} from './types.ts';
import type {AttachedSession} from '../sessions/types.ts';
import {restoreView, editorView} from './view-state.ts';
import type {State} from '../state.ts';

export const emptySession = (id: string): AttachedSession => ({id, transcript: {order: [], items: {}}, transcriptReady: false, expanded: {}});

export const emptyEditor = (target: EditorTarget): EditorView => ({
  id: targetKey(target), target, activePane: null, panel: false,
  tabs: 'workspace' in target ? dashboardTabs : agentTabs, scroll: {}, changes: null, diffs: {},
  files: {listings: {}, listingErrors: {}, expanded: {}, open: {}},
});

// Attachment separates shared file tools from the real session transcript.
export function attach(state: State): State {
  const target = selectionTarget(state.selected);
  const id = target && targetExists(state, target) ? targetKey(target) : null;
  if (id === (state.editor?.id ?? null)) return state;
  const sessionViews = {...state.sessionViews};
  const workspaceViews = {...state.workspaceViews};
  const fileTrees = {...state.fileTrees};
  const previous = state.editor;
  if (previous && targetExists(state, previous.target)) {
    const saved = editorView(previous);
    if ('workspace' in previous.target) workspaceViews[previous.target.workspace] = saved;
    else sessionViews[previous.target.session] = {...saved, expanded: state.session!.expanded};
    fileTrees[previous.id] = {listings: previous.files.listings, changes: previous.changes};
  }
  const sessionView = target && 'session' in target ? sessionViews[target.session] : undefined;
  const saved = target && ('workspace' in target ? workspaceViews[target.workspace] : sessionView);
  if (target) {
    if ('workspace' in target) delete workspaceViews[target.workspace];
    else delete sessionViews[target.session];
  }
  const cached = id ? fileTrees[id] : undefined;
  if (id) delete fileTrees[id];
  let editor = id && target ? emptyEditor(target) : null;
  if (editor && saved) editor = restoreView(editor, saved);
  if (editor && cached) editor = {...editor, changes: cached.changes, files: {...editor.files, listings: cached.listings}};
  let session: AttachedSession | null = null;
  if (editor && target && 'session' in target) {
    session = {...emptySession(target.session), expanded: sessionView?.expanded ?? {}};
  }
  return {...state, sessionViews, workspaceViews, fileTrees, session, editor, editorVersion: state.editorVersion + 1, treeVersion: state.treeVersion + 1,
    fileReadVersions: {}, diffReadVersions: {}};
}
