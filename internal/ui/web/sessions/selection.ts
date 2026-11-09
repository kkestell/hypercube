// Navigation attaches either a session or workspace to the shared editor.
// Only a real session also attaches a transcript.
// The unsent session is a new session the user has opened but not yet sent a
// first prompt to; it exists only in this page.
import {allSessions, sessionOf} from './lookup.ts';
export {allSessions, sessionOf} from './lookup.ts';
import {draftDefaults, isUnsent, unsentKey, unsentOf, withDraftDefaults} from '../agent/drafts.ts';
import {attach} from '../editor/attachment.ts';
export {emptySession, emptyEditor} from '../editor/attachment.ts';
import {selectedWorkspace, selectionTarget, targetExists, workspaceKey} from '../editor/target.ts';
import type {Settings} from '../agent/types.ts';
import type {State} from '../state.ts';
import type {Session, Unsent, Workspace} from './types.ts';

export const waitingCount = (state: State) => {
  const waiting = new Set(state.requests.map(request => request.session));
  return allSessions(state).filter(s => !s.ended && (s.attention || waiting.has(s.id))).length;
};

export const currentSession = (state: State) => allSessions(state).find(s => s.id === state.selected);

// The unsent session takes the harness of its settings.
const unsentSession = (id: string, sending: boolean, harness = ''): Session => ({
  id, parent: '', title: 'New Session', ready: false, ended: false, unseen: false,
  harness, model: '', effort: '', dir: '', activity: sending ? 'busy' : 'idle', attention: false, usage: {},
  worktree: false, children: [],
});

const placeholder = (state: State, unsent: Unsent) =>
  unsentSession(unsentKey(unsent.workspace), unsent.sending, unsent.settings?.harness);


// shownSession is the unsent placeholder or the attached session.
export function shownSession(state: State): Session | null {
  if (state.unsent) return placeholder(state, state.unsent);
  return state.session ? currentSession(state) ?? null : null;
}

// shownId names the shown unsent session or the selected session.
export const shownId = (state: State) => state.unsent ? unsentKey(state.unsent.workspace) : state.selected;

// unsentRow is the placeholder for the workspace's unsent session, shown or
// parked, which the sidebar lists above its sessions.
export function unsentRow(state: State, workspace: string): Session | null {
  const unsent = unsentOf(state, unsentKey(workspace));
  return unsent ? placeholder(state, unsent) : null;
}

export function pageTitle(state: State) {
  const waiting = waitingCount(state);
  const session = currentSession(state);
  const {unsent} = state;
  const workspace = unsent ? state.workspaces.find(w => w.id === unsent.workspace) : selectedWorkspace(state);
  return (waiting ? `(${waiting}) ` : '') + [session?.title, workspace?.name, 'Hypercube'].filter(Boolean).join(' — ');
}

const omit = <T>(object: Record<string, T>, keys: string[]) => Object.fromEntries(Object.entries(object).filter(([key]) => !keys.includes(key)));

// parkUnsent keeps the shown unsent session when it has a draft or is
// sending, and drops it otherwise.
function parkUnsent(state: State): State {
  if (!state.unsent) return state;
  const key = unsentKey(state.unsent.workspace);
  if (!state.drafts[key] && !state.unsent.sending && !state.submissions[key]?.length) return {...state, unsent: null};
  return {...state, unsent: null, unsentDrafts: {...state.unsentDrafts, [key]: state.unsent}};
}

// receivedWorkspaces applies a state message's workspaces.
export function receivedWorkspaces(state: State, workspaces: Workspace[]): State {
  let next = {...state, workspaces};
  const removed = (draft: Unsent) => draft.workspace !== null && !workspaces.some(w => w.id === draft.workspace);
  const discarded = Object.entries(next.unsentDrafts).filter(([, draft]) => removed(draft)).map(([key]) => key);
  if (next.unsent && removed(next.unsent)) {
    discarded.push(unsentKey(next.unsent.workspace));
    next = {...next, unsent: null};
  }
  if (discarded.length) next = {...next, drafts: omit(next.drafts, discarded), unsentDrafts: omit(next.unsentDrafts, discarded)};
  const sessions = allSessions(next);
  const ids = new Set(sessions.map(session => session.id));
  const removedIDs = [...discarded, ...allSessions(state).filter(session => !ids.has(session.id)).map(session => session.id)];
  next = {...next, submissions: omit(next.submissions, removedIDs)};

  const workspaceIds = new Set(workspaces.map(workspace => workspace.id));
  const fileTrees = Object.fromEntries(Object.entries(next.fileTrees).filter(([key]) => {
    const target = selectionTarget(key);
    return target && targetExists(next, target);
  }));
  next = {...next,
    sessionViews: Object.fromEntries(Object.entries(next.sessionViews).filter(([id]) => ids.has(id))),
    workspaceViews: Object.fromEntries(Object.entries(next.workspaceViews).filter(([id]) => workspaceIds.has(id))),
    fileTrees, workbench: {
    ...next.workbench,
    collapsedWorkspaces: Object.fromEntries(Object.entries(next.workbench.collapsedWorkspaces).filter(([id]) => workspaceIds.has(id))),
    collapsedSessions: Object.fromEntries(Object.entries(next.workbench.collapsedSessions).filter(([id]) => ids.has(id))),
  }};
  if (next.pending && sessions.some(s => s.id === next.pending)) {
    const family = sessionOf(next, next.pending)!;
    next = {...next, selected: next.pending, pending: null, workbench: {...next.workbench, sidebarTab: (family.parent ?? family.session).ended ? 'archived' : 'sessions'}};
  }
  // A family archived while shown is deselected, and nothing takes its place.
  // Otherwise a selection naming no session moves to one that has not ended.
  const target = selectionTarget(next.selected);
  const chosen = currentSession(next);
  const family = sessionOf(next, next.selected);
  const oldFamily = sessionOf(state, state.selected);
  if (family && (family.parent ?? family.session).ended && !(oldFamily?.parent ?? oldFamily?.session)?.ended && next.session?.id === chosen?.id) next = {...next, selected: null};
  else if (target && 'workspace' in target) {
    if (!selectedWorkspace(next)) next = {...next, selected: null};
  } else if (next.selected !== null && !chosen) next = {...next, selected: sessions.find(s => !s.ended)?.id ?? ''};
  return attach(withDraftDefaults(next));
}

// selectSession shows a session or an unsent session. A new session's ID can
// arrive before the state that lists it, so the selection waits for that state.
export const selectWorkspace = (state: State, id: string) => selectSession(state, workspaceKey(id));

export function selectSession(state: State, id: string): State {
  if (isUnsent(id)) return openUnsent(state, unsentOf(state, id)!.workspace);
  const next = parkUnsent(state);
  const target = selectionTarget(id);
  if (target && 'workspace' in target) return targetExists(next, target) ? attach({...next, selected: id, pending: null}) : next;
  if (!allSessions(next).some(s => s.id === id)) return {...next, pending: id};
  const family = sessionOf(next, id)!;
  return attach({...next, selected: id, pending: null, workbench: {...next.workbench, sidebarTab: (family.parent ?? family.session).ended ? 'archived' : 'sessions'}});
}

// showHome shows the home view: nothing selected, so the center lists the
// pull requests that need the user.
export const showHome = (state: State): State => attach({...parkUnsent(state), selected: null, pending: null});

// homeShown is whether the center is the home view.
export const homeShown = (state: State) => state.selected === null && !state.unsent && !state.editor;

// openUnsent reopens the workspace's unfinished session or creates one.
export function openUnsent(state: State, workspace?: string | null): State {
  workspace ??= state.unsent?.workspace ?? selectedWorkspace(state)?.id ?? state.workspaces[0]?.id ?? null;
  if (state.unsent?.workspace === workspace) return state.workbench.sidebarTab === 'sessions' ? state : {...state, workbench: {...state.workbench, sidebarTab: 'sessions'}};
  const next = parkUnsent(state);
  const key = unsentKey(workspace);
  const unsentDrafts = omit(next.unsentDrafts, [key]);
  // Identity survives immutable updates to the sending flag.
  const unsent: Unsent = next.unsentDrafts[key] ?? {identity: {}, workspace, settings: draftDefaults(next, workspace), worktree: null, sending: false};
  return attach({...next, unsent, unsentDrafts, selected: null, pending: null, workbench: {...next.workbench, sidebarTab: 'sessions'}});
}

// unsentSettingsChosen sets the unsent session's settings.
export const unsentSettingsChosen = (state: State, settings: Settings): State => state.unsent
  ? {...state, unsent: {...state.unsent, settings}}
  : state;

// inWorktree is whether the unsent session will run in its own worktree. A
// workspace without Git always runs locally.
export function inWorktree(state: State, unsent: Unsent) {
  if (state.workspaces.find(w => w.id === unsent.workspace)?.noGit) return false;
  return unsent.worktree ?? state.config?.default_worktree ?? false;
}

// unsentWorktree sets whether the unsent session runs in its own worktree.
export const unsentWorktree = (state: State, worktree: boolean): State => state.unsent
  ? {...state, unsent: {...state.unsent, worktree}}
  : state;

export function unsentSending(state: State, sending: boolean, key = state.unsent && unsentKey(state.unsent.workspace)): State {
  if (!key) return state;
  if (state.unsent && unsentKey(state.unsent.workspace) === key) return {...state, unsent: {...state.unsent, sending}};
  const draft = state.unsentDrafts[key];
  return draft ? {...state, unsentDrafts: {...state.unsentDrafts, [key]: {...draft, sending}}} : state;
}
