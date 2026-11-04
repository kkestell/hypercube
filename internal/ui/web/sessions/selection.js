// Which session the page shows. The server's workspaces list the sessions;
// selected names the chosen one, and the page attaches to it,
// which gives it a fresh session state for its transcripts, tabs, and files.
// The unsent session is a new session the user has opened but not yet sent a
// first prompt to; it exists only in this page.
import {unsentKey} from '../agent/drafts.js';
import {agentTabs, openBrowserTab} from '../editor/editor.js';

export const allSessions = state => state.workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children]));
export const waitingCount = state => allSessions(state).filter(s => s.attention).length;

const hasRunningAgent = session => (session.running && !session.ended) || session.children.some(child => child.running && !child.ended);
export const runningFirst = sessions => sessions.toSorted((a, b) => Number(hasRunningAgent(b)) - Number(hasRunningAgent(a)));

export const currentSession = state => allSessions(state).find(s => s.id === state.selected);

// sessionOf finds a session, its workspace, and its parent when it is a child.
export function sessionOf(state, id) {
  for (const workspace of state.workspaces) {
    for (const top of workspace.sessions) {
      if (top.id === id) return {workspace, session: top, parent: null};
      const child = top.children.find(s => s.id === id);
      if (child) return {workspace, session: child, parent: top};
    }
  }
}

export const failed = session => !session.parent && session.running && session.activity === 'exited';

// shownSession is the unsent placeholder or the attached session.
export function shownSession(state) {
  if (state.unsent) return {
    id: unsentKey, activity: state.unsent.sending ? 'busy' : 'idle',
  };
  return state.session ? currentSession(state) : null;
}

export function pageTitle(state) {
  const waiting = waitingCount(state);
  const session = currentSession(state);
  const workspace = state.unsent ? state.workspaces.find(w => w.id === state.unsent.workspace) : sessionOf(state, session?.id)?.workspace;
  return (waiting ? `(${waiting}) ` : '') + [session?.title, workspace?.name, 'Hypercube'].filter(Boolean).join(' — ');
}

export const emptySession = id => ({
  id,
  transcript: {order: [], items: {}},
  expanded: {},
  tabs: agentTabs,
  scroll: {},
  changes: null,
  diffs: {},
  files: {listings: {}, expanded: {}, open: {}},
});

const omit = (object, keys) => Object.fromEntries(Object.entries(object).filter(([key]) => !keys.includes(key)));

function discardUnsent(state) {
  if (!state.unsent) return state;
  return {...state, unsent: null, drafts: omit(state.drafts, [unsentKey])};
}

// attach replaces the session state when the shown session changes. The page
// attaches to the selected session in any state, with its browser's tab open
// if it has a browser.
function attach(state) {
  const shown = currentSession(state);
  const id = shown?.id ?? null;
  if (id === (state.session?.id ?? null)) return state;
  const next = {...state, session: id ? emptySession(id) : null};
  return state.browsers?.[id] ? openBrowserTab(next, false) : next;
}

// receivedWorkspaces applies a state message's workspaces.
export function receivedWorkspaces(state, workspaces) {
  let next = {...state, workspaces};
  if (next.unsent?.workspace && !workspaces.some(w => w.id === next.unsent.workspace)) next = discardUnsent(next);
  const sessions = allSessions(next);
  if (next.pending && sessions.some(s => s.id === next.pending)) next = {...next, selected: next.pending, pending: null};
  // A session that stops while shown is deselected, and nothing takes its
  // place. Otherwise a selection naming no session moves to a running one.
  const chosen = currentSession(next);
  const family = sessionOf(next, next.selected);
  const oldFamily = sessionOf(state, state.selected);
  if (family && (family.parent ?? family.session).ended && !(oldFamily?.parent ?? oldFamily?.session)?.ended && next.session?.id === chosen.id) next = {...next, selected: null};
  else if (chosen && !chosen.running && !chosen.ended && currentSession(state)?.running && next.session?.id === chosen.id) next = {...next, selected: null};
  else if (next.selected !== null && !chosen) next = {...next, selected: sessions.find(s => s.running)?.id ?? ''};
  return attach(next);
}

// selectSession shows a session. A new session's ID can arrive before the
// state that lists it, so the selection waits for that state.
export function selectSession(state, id) {
  const next = discardUnsent(state);
  if (!allSessions(next).some(s => s.id === id)) return {...next, pending: id};
  return attach({...next, selected: id, pending: null});
}

// openUnsent opens a new session in a workspace, in place of any other unsent
// session. It has no profile until the composer chooses one, and runs in the
// checkout unless the composer chooses a worktree.
export function openUnsent(state, workspace) {
  workspace ??= state.unsent?.workspace ?? sessionOf(state, state.selected)?.workspace.id ?? state.workspaces[0]?.id ?? null;
  const next = discardUnsent(state);
  // Identity survives immutable updates to the sending flag.
  const unsent = {identity: {}, workspace, profile: null, worktree: false, sending: false};
  return attach({...next, unsent, selected: null, pending: null});
}

// unsentProfile sets the unsent session's profile.
export const unsentProfile = (state, profile) => state.unsent
  ? {...state, unsent: {...state.unsent, profile: profile.name}}
  : state;

// unsentWorktree sets whether the unsent session runs in its own worktree.
export const unsentWorktree = (state, worktree) => state.unsent
  ? {...state, unsent: {...state.unsent, worktree}}
  : state;

export const unsentSending = (state, sending) => state.unsent ? {...state, unsent: {...state.unsent, sending}} : state;
