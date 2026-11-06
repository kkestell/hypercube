// Which session the page shows. The server's workspaces list the sessions;
// selected names the chosen one, and the page attaches to it,
// which gives it a fresh session state for its transcripts, tabs, and files.
// The unsent session is a new session the user has opened but not yet sent a
// first prompt to; it exists only in this page.
import {isUnsent, unsentKey, unsentOf} from '../agent/drafts.ts';
import {agentTabs} from '../editor/editor.ts';
import type {Profile} from '../agent/types.ts';
import type {State} from '../state.ts';
import type {AttachedSession, Session, Unsent, Workspace} from './types.ts';
import {restoreView, sessionView} from './view-state.ts';

export const allSessions = (state: State): Session[] => state.workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children]));
export const waitingCount = (state: State) => {
  const waiting = new Set(state.requests.map(request => request.session));
  return allSessions(state).filter(s => !s.ended && (s.attention || waiting.has(s.id))).length;
};

export const currentSession = (state: State) => allSessions(state).find(s => s.id === state.selected);

// sessionOf finds a session, its workspace, and its parent when it is a child.
export function sessionOf(state: State, id: string | null): {workspace: Workspace; session: Session; parent: Session | null} | undefined {
  for (const workspace of state.workspaces) {
    for (const top of workspace.sessions) {
      if (top.id === id) return {workspace, session: top, parent: null};
      const child = top.children.find(s => s.id === id);
      if (child) return {workspace, session: child, parent: top};
    }
  }
}

// The unsent session takes the harness of its chosen profile.
const unsentSession = (id: string, sending: boolean, harness = ''): Session => ({
  id, parent: '', title: 'New Session', ready: false, ended: false, unseen: false,
  harness, model: '', effort: '', dir: '', activity: sending ? 'busy' : 'idle', attention: false, usage: {},
  worktree: false, children: [],
});

const placeholder = (state: State, unsent: Unsent) =>
  unsentSession(unsentKey(unsent.workspace), unsent.sending, state.config?.profiles.find(p => p.name === unsent.profile)?.harness);

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
  const workspace = unsent ? state.workspaces.find(w => w.id === unsent.workspace) : session && sessionOf(state, session.id)?.workspace;
  return (waiting ? `(${waiting}) ` : '') + [session?.title, workspace?.name, 'Hypercube'].filter(Boolean).join(' — ');
}

export const emptySession = (id: string): AttachedSession => ({
  id,
  transcript: {order: [], items: {}},
  transcriptReady: false,
  activePane: null,
  expanded: {},
  tabs: agentTabs,
  scroll: {},
  changes: null,
  diffs: {},
  files: {listings: {}, expanded: {}, open: {}},
});

const omit = <T>(object: Record<string, T>, keys: string[]) => Object.fromEntries(Object.entries(object).filter(([key]) => !keys.includes(key)));

// parkUnsent keeps the shown unsent session when it has a draft or is
// sending, and drops it otherwise.
function parkUnsent(state: State): State {
  if (!state.unsent) return state;
  const key = unsentKey(state.unsent.workspace);
  if (!state.drafts[key] && !state.unsent.sending) return {...state, unsent: null};
  return {...state, unsent: null, unsentDrafts: {...state.unsentDrafts, [key]: state.unsent}};
}

// attach replaces the session state when the shown session changes.
function attach(state: State): State {
  const shown = currentSession(state);
  const id = shown?.id ?? null;
  if (id === (state.session?.id ?? null)) return state;
  const sessionViews = {...state.sessionViews};
  const previous = state.session;
  if (previous && allSessions(state).some(session => session.id === previous.id)) {
    sessionViews[previous.id] = sessionView(previous);
  }
  const saved = id ? sessionViews[id] : undefined;
  if (id) delete sessionViews[id];
  const session = id ? emptySession(id) : null;
  return {...state, sessionViews, session: session && saved ? restoreView(session, saved) : session};
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
  const sessionViews = Object.fromEntries(Object.entries(next.sessionViews).filter(([id]) => ids.has(id)));
  const workspaceIds = new Set(workspaces.map(workspace => workspace.id));
  next = {...next, sessionViews, workbench: {
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
  const chosen = currentSession(next);
  const family = sessionOf(next, next.selected);
  const oldFamily = sessionOf(state, state.selected);
  if (family && (family.parent ?? family.session).ended && !(oldFamily?.parent ?? oldFamily?.session)?.ended && next.session?.id === chosen?.id) next = {...next, selected: null};
  else if (next.selected !== null && !chosen) next = {...next, selected: sessions.find(s => !s.ended)?.id ?? ''};
  return attach(next);
}

// selectSession shows a session or an unsent session. A new session's ID can
// arrive before the state that lists it, so the selection waits for that state.
export function selectSession(state: State, id: string): State {
  if (isUnsent(id)) return openUnsent(state, unsentOf(state, id)!.workspace);
  const next = parkUnsent(state);
  if (!allSessions(next).some(s => s.id === id)) return {...next, pending: id};
  const family = sessionOf(next, id)!;
  return attach({...next, selected: id, pending: null, workbench: {...next.workbench, sidebarTab: (family.parent ?? family.session).ended ? 'archived' : 'sessions'}});
}

// openUnsent reopens the workspace's unfinished session or creates one.
export function openUnsent(state: State, workspace?: string | null): State {
  workspace ??= state.unsent?.workspace ?? sessionOf(state, state.selected)?.workspace.id ?? state.workspaces[0]?.id ?? null;
  if (state.unsent?.workspace === workspace) return state.workbench.sidebarTab === 'sessions' ? state : {...state, workbench: {...state.workbench, sidebarTab: 'sessions'}};
  const next = parkUnsent(state);
  const key = unsentKey(workspace);
  const unsentDrafts = omit(next.unsentDrafts, [key]);
  // Identity survives immutable updates to the sending flag.
  const unsent: Unsent = next.unsentDrafts[key] ?? {identity: {}, workspace, profile: null, worktree: null, sending: false};
  return attach({...next, unsent, unsentDrafts, selected: null, pending: null, workbench: {...next.workbench, sidebarTab: 'sessions'}});
}

// unsentProfile sets the unsent session's profile.
export const unsentProfile = (state: State, profile: Profile): State => state.unsent
  ? {...state, unsent: {...state.unsent, profile: profile.name}}
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
