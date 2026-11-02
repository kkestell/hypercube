// Which session the page shows. The server's workspaces list the sessions;
// selected names the chosen one, and the page attaches to it while it runs,
// which gives it a fresh session state for its transcripts, tabs, and files.
// The unsent session is a new session the user has opened but not yet sent a
// first prompt to; it exists only in this page.
import {closeTab} from '../editor/tab-layout.js';

// The unsent session's lead is drawn under this ID and keeps its draft here.
export const unsentKey = 'new';

export const allSessions = state => state.workspaces.flatMap(w => w.sessions);

export const currentSession = state => allSessions(state).find(s => s.id === state.selected);

// sessionOfAgent finds an agent with its session and workspace.
export function sessionOfAgent(state, id) {
  for (const workspace of state.workspaces) {
    for (const session of workspace.sessions) {
      const agent = session.agents.find(a => a.id === id);
      if (agent) return {workspace, session, agent};
    }
  }
  return undefined;
}

// A running session whose lead has exited, as after a failed start, can be
// started again.
export const failed = session => session.running && session.agents.some(a => a.role === 'lead' && a.activity === 'exited');

const unsentLead = unsent => ({
  id: unsentKey, name: 'lead', role: 'lead', activity: unsent.sending ? 'busy' : 'idle',
  harness: unsent.harness, model: unsent.model, effort: unsent.effort,
});

// shownAgents are the agents drawn in the center: the unsent session's lead,
// or the attached session's agents.
export function shownAgents(state) {
  if (state.unsent) return [unsentLead(state.unsent)];
  return state.session ? currentSession(state)?.agents ?? [] : [];
}

export function pageTitle(state) {
  const waiting = allSessions(state).filter(s => s.running).reduce((n, s) => n + s.agents.filter(a => a.attention).length, 0);
  const session = currentSession(state);
  const workspace = state.unsent ? state.workspaces.find(w => w.id === state.unsent.workspace) : state.workspaces.find(w => w.sessions.includes(session));
  return (waiting ? `(${waiting}) ` : '') + [session?.title, workspace?.name, 'Hypercube'].filter(Boolean).join(' — ');
}

export const emptySession = id => ({
  id,
  transcripts: {},
  expanded: {},
  tabs: {root: null, preview: null, dragging: null, nextId: 1},
  scroll: {},
  files: {listings: {}, expanded: {}, open: {}},
});

const omit = (object, keys) => Object.fromEntries(Object.entries(object).filter(([key]) => !keys.includes(key)));

function discardUnsent(state) {
  if (!state.unsent) return state;
  return {...state, unsent: null, drafts: omit(state.drafts, [unsentKey])};
}

// attach replaces the session state when the shown session changes. The page
// attaches to the selected session while it runs.
function attach(state) {
  const shown = currentSession(state);
  const id = shown?.running ? shown.id : null;
  if (id === (state.session?.id ?? null)) return state;
  return {...state, session: id ? emptySession(id) : null};
}

// withoutDeparted drops the transcripts, tabs, and scroll of agents that left
// the attached session.
function withoutDeparted(before, after) {
  const session = after.session;
  if (!session || session !== before.session) return after;
  const present = new Set(currentSession(after).agents.map(a => a.id));
  const gone = (currentSession(before)?.agents ?? []).map(a => a.id).filter(id => !present.has(id));
  if (!gone.length) return after;
  return {
    ...after,
    session: {
      ...session,
      transcripts: omit(session.transcripts, gone),
      scroll: omit(session.scroll, gone),
      tabs: gone.reduce(closeTab, session.tabs),
    },
  };
}

// receivedWorkspaces applies a state message's workspaces.
export function receivedWorkspaces(state, workspaces) {
  let next = {...state, workspaces};
  if (next.unsent && !workspaces.some(w => w.id === next.unsent.workspace)) next = discardUnsent(next);
  const sessions = allSessions(next);
  if (next.pending && sessions.some(s => s.id === next.pending)) next = {...next, selected: next.pending, pending: null};
  // A session that stops while shown is deselected, and nothing takes its
  // place. Otherwise a selection naming no session moves to a running one.
  const chosen = currentSession(next);
  if (chosen && !chosen.running && next.session?.id === chosen.id) next = {...next, selected: null};
  else if (next.selected !== null && !chosen) next = {...next, selected: sessions.find(s => s.running)?.id ?? ''};
  return withoutDeparted(state, attach(next));
}

// selectSession shows a session. A new session's ID can arrive before the
// state that lists it, so the selection waits for that state.
export function selectSession(state, id) {
  const next = discardUnsent(state);
  if (!allSessions(next).some(s => s.id === id)) return {...next, pending: id};
  return attach({...next, selected: id, pending: null});
}

// openUnsent opens a new session in a workspace with a lead profile, in place
// of any other unsent session.
export function openUnsent(state, workspace, profile) {
  const next = discardUnsent(state);
  // Identity survives immutable updates to the sending flag.
  const unsent = {identity: {}, workspace, lead: profile.name, harness: profile.harness, model: profile.model, effort: profile.effort, sending: false};
  return attach({...next, unsent, selected: null, pending: null});
}

export const unsentSending = (state, sending) => state.unsent ? {...state, unsent: {...state.unsent, sending}} : state;

// unsentCreated gives the created session's lead whatever the user typed in
// the unsent session while its first prompt was sent.
export function unsentCreated(state, lead) {
  const draft = state.drafts[unsentKey];
  if (!draft) return state;
  const drafts = omit(state.drafts, [unsentKey]);
  // Pending reads belong to the discarded unsent session and will be ignored.
  if (draft.text || draft.images.length) drafts[lead] = {...draft, reading: 0};
  return {...state, drafts};
}
