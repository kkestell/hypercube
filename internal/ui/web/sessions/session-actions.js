// What the user does to sessions and workspaces. The server applies each
// action, and the result arrives in the next state message.
import {post} from '../api.js';
import {allSessions, failed, selectSession, unsentCreated, unsentSending} from './selection.js';

export const startSession = id => post('session', {action: 'start', id});
export const stopSession = id => post('session', {action: 'stop', id});
export const renameSession = (id, title) => post('session', {action: 'rename', id, title});
export const markSeen = id => post('session', {action: 'seen', id});
export const addWorkspace = () => post('workspace', {action: 'new'});

export function deleteSession(session) {
  if (confirm(`Delete session ${session.title}?`)) post('session', {action: 'delete', id: session.id});
}

export function deleteWorkspace(workspace) {
  if (confirm(`Delete workspace ${workspace.name}?`)) post('workspace', {action: 'delete', id: workspace.id});
}

// chooseSession shows a session, starting it if it is stopped or its lead
// failed to start.
export function chooseSession(store, id) {
  store.update(state => selectSession(state, id));
  const session = allSessions(store.get()).find(s => s.id === id);
  if (session && (!session.running || failed(session))) startSession(id);
}

// createSession sends the unsent session's first prompt, which creates the
// session, then shows it. It reports whether the server accepted the prompt.
export async function createSession(store, prompt) {
  const {workspace, lead} = store.get().unsent;
  store.update(state => unsentSending(state, true));
  const created = await post('session', {action: 'new', workspace, lead}, JSON.stringify(prompt));
  store.update(state => unsentSending(state, false));
  if (created === null) return false;
  // The user may have closed the unsent session meanwhile.
  if (!store.get().unsent) return true;
  const ids = JSON.parse(created);
  store.update(state => unsentCreated(state, ids.lead));
  chooseSession(store, ids.session);
  return true;
}
