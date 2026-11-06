// Which session the page shows. The server's workspaces list the sessions;
// selected names the chosen one, and the page attaches to it,
// which gives it a fresh session state for its transcripts, tabs, and files.
// The unsent session is a new session the user has opened but not yet sent a
// first prompt to; it exists only in this page.
import {unsentKey} from '../agent/drafts.ts';
import {agentTabs} from '../editor/editor.ts';
import type {Profile} from '../agent/types.ts';
import type {State} from '../state.ts';
import type {AttachedSession, Session, Unsent, Workspace} from './types.ts';

export const allSessions = (state: State): Session[] => state.workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children]));
export const waitingCount = (state: State) => allSessions(state).filter(s => s.attention).length;

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
const unsentSession = (sending: boolean, harness = ''): Session => ({
  id: unsentKey, parent: '', title: 'New Session', ready: false, ended: false, unseen: false,
  harness, model: '', effort: '', dir: '', activity: sending ? 'busy' : 'idle', attention: false, usage: {},
  worktree: false, children: [],
});

// shownSession is the unsent placeholder or the attached session.
export function shownSession(state: State): Session | null {
  if (state.unsent) {
    const {sending, profile} = state.unsent;
    return unsentSession(sending, state.config?.profiles.find(p => p.name === profile)?.harness);
  }
  return state.session ? currentSession(state) ?? null : null;
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
  expanded: {},
  tabs: agentTabs,
  scroll: {},
  changes: null,
  diffs: {},
  files: {listings: {}, expanded: {}, open: {}},
});

const omit = <T>(object: Record<string, T>, keys: string[]) => Object.fromEntries(Object.entries(object).filter(([key]) => !keys.includes(key)));

function discardUnsent(state: State): State {
  if (!state.unsent) return state;
  return {...state, unsent: null, drafts: omit(state.drafts, [unsentKey])};
}

// attach replaces the session state when the shown session changes.
function attach(state: State): State {
  const shown = currentSession(state);
  const id = shown?.id ?? null;
  if (id === (state.session?.id ?? null)) return state;
  return {...state, session: id ? emptySession(id) : null};
}

// receivedWorkspaces applies a state message's workspaces.
export function receivedWorkspaces(state: State, workspaces: Workspace[]): State {
  let next = {...state, workspaces};
  const unsentWorkspace = next.unsent?.workspace;
  if (unsentWorkspace && !workspaces.some(w => w.id === unsentWorkspace)) next = discardUnsent(next);
  const sessions = allSessions(next);
  if (next.pending && sessions.some(s => s.id === next.pending)) next = {...next, selected: next.pending, pending: null};
  // A family archived while shown is deselected, and nothing takes its place.
  // Otherwise a selection naming no session moves to one that has not ended.
  const chosen = currentSession(next);
  const family = sessionOf(next, next.selected);
  const oldFamily = sessionOf(state, state.selected);
  if (family && (family.parent ?? family.session).ended && !(oldFamily?.parent ?? oldFamily?.session)?.ended && next.session?.id === chosen?.id) next = {...next, selected: null};
  else if (next.selected !== null && !chosen) next = {...next, selected: sessions.find(s => !s.ended)?.id ?? ''};
  return attach(next);
}

// selectSession shows a session. A new session's ID can arrive before the
// state that lists it, so the selection waits for that state.
export function selectSession(state: State, id: string): State {
  const next = discardUnsent(state);
  if (!allSessions(next).some(s => s.id === id)) return {...next, pending: id};
  return attach({...next, selected: id, pending: null});
}

// openUnsent opens a new session in a workspace, in place of any other unsent
// session. It has no profile until the composer chooses one, and runs in the
// checkout unless the composer chooses a worktree.
export function openUnsent(state: State, workspace?: string | null): State {
  workspace ??= state.unsent?.workspace ?? sessionOf(state, state.selected)?.workspace.id ?? state.workspaces[0]?.id ?? null;
  const next = discardUnsent(state);
  // Identity survives immutable updates to the sending flag.
  const unsent: Unsent = {identity: {}, workspace, profile: null, worktree: false, sending: false};
  return attach({...next, unsent, selected: null, pending: null});
}

// unsentProfile sets the unsent session's profile.
export const unsentProfile = (state: State, profile: Profile): State => state.unsent
  ? {...state, unsent: {...state.unsent, profile: profile.name}}
  : state;

// unsentWorktree sets whether the unsent session runs in its own worktree.
export const unsentWorktree = (state: State, worktree: boolean): State => state.unsent
  ? {...state, unsent: {...state.unsent, worktree}}
  : state;

export const unsentSending = (state: State, sending: boolean): State => state.unsent ? {...state, unsent: {...state.unsent, sending}} : state;
