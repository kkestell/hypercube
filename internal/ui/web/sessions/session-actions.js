// What the user does to sessions and workspaces, and the prompts they send.
// The server applies each action, and the result arrives in the next state
// message.
import {post} from '../api.js';
import {canSend, cleared, draftOf, restored, unsentCreated, unsentKey} from '../agent/drafts.js';
import {askToConfirm} from '../shared/confirm.js';
import {allSessions, failed, selectSession, unsentSending} from './selection.js';

export const startSession = (store, id) => post(store, 'session', {action: 'start', id});
export const stopSession = (store, id) => post(store, 'session', {action: 'stop', id});
export const renameSession = (store, id, title) => post(store, 'session', {action: 'rename', id, title});
export const markSeen = (store, id) => post(store, 'session', {action: 'seen', id});
export const addWorkspace = store => post(store, 'workspace', {action: 'new'});

const confirmDelete = (store, name, detail, accept) =>
  askToConfirm(store, {message: `Are you sure you want to delete '${name}'?`, detail, primary: 'Delete', accept});

export const deleteSession = (store, session) => confirmDelete(store, session.title,
  'Its agents stop, and its chats and its workers\' worktrees and branches are deleted.',
  () => post(store, 'session', {action: 'delete', id: session.id}));

export const deleteWorkspace = (store, workspace) => confirmDelete(store, workspace.name,
  'Its sessions are deleted with their chats and their workers\' worktrees and branches.',
  () => post(store, 'workspace', {action: 'delete', id: workspace.id}));

// chooseSession shows a session, starting it if it is stopped or its lead
// failed to start.
export function chooseSession(store, id) {
  store.update(state => selectSession(state, id));
  const session = allSessions(store.get()).find(s => s.id === id);
  if (session && (!session.running || failed(session))) startSession(store, id);
}

// createSession sends the unsent session's first prompt, which creates the
// session, then shows it. A refusal restores the prompt only while that
// unsent session is still open.
export async function createSession(store, prompt) {
  store.update(state => unsentSending(state, true));
  const unsent = store.get().unsent;
  const {workspace, lead} = unsent;
  const created = await post(store, 'session', {action: 'new', workspace, lead}, JSON.stringify(prompt));
  // A completion must not change another unsent session or restore into it.
  if (store.get().unsent?.identity !== unsent.identity) return true;
  store.update(state => unsentSending(state, false));
  if (created === null) return false;
  const ids = JSON.parse(created);
  store.update(state => unsentCreated(state, ids.lead));
  chooseSession(store, ids.session);
  return true;
}

// sendPrompt sends an agent's draft. The unsent session's first prompt
// creates the session. A refused prompt returns to its draft.
export async function sendPrompt(store, agent) {
  const state = store.get();
  if (!canSend(state, agent)) return;
  const {text, images} = draftOf(state, agent.id);
  store.update(next => cleared(next, agent.id));
  const accepted = agent.id === unsentKey
    ? await createSession(store, {text, images})
    : await post(store, 'agent', {action: 'input', id: agent.id}, JSON.stringify({text, images})) !== null;
  if (!accepted) store.update(next => restored(next, agent.id, {text, images}));
}
