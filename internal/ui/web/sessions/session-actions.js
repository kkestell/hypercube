// What the user does to sessions and workspaces, and the prompts they send.
// The server applies each action, and the result arrives in the next state
// message.
import {post} from '../api.js';
import {canSend, cleared, draftOf, restored, unsentCreated, unsentKey} from '../agent/drafts.js';
import {askToConfirm} from '../shared/confirm.js';
import {allSessions, failed, openUnsent, selectSession, unsentSending} from './selection.js';

export const startSession = (store, id) => post(store, 'session', {action: 'start', id});
export const archiveSession = (store, id) => post(store, 'session', {action: 'archive', id});
export const stopSession = (store, id) => post(store, 'session', {action: 'stop', id});
export const renameSession = (store, id, title) => post(store, 'session', {action: 'rename', id, title});
export const markSeen = (store, id) => post(store, 'session', {action: 'seen', id});
// addWorkspace opens a new session in the added workspace.
export async function addWorkspace(store) {
  const workspace = await post(store, 'workspace', {action: 'new'});
  if (workspace) store.update(s => openUnsent(s, workspace));
}

const confirmDelete = (store, name, detail, accept) =>
  askToConfirm(store, {message: `Are you sure you want to delete '${name}'?`, detail, primary: 'Delete', accept});

const deleteDetail = session => {
  if (session.ended) return "Its chat is deleted.";
  if (session.worktree) {
    const branch = session.merge ? ` and branch ${session.merge.branch}` : '';
    return `Its agent stops, and its chat, its worktree ${session.dir}${branch}, and its children and their worktrees and branches are deleted.`;
  }
  return "Its agent stops, and its chat, children, and their worktrees and branches are deleted.";
};

export const deleteSession = (store, session) => confirmDelete(store, session.title, deleteDetail(session),
  () => post(store, 'session', {action: 'delete', id: session.id}));

export const deleteWorkspace = (store, workspace) => confirmDelete(store, workspace.name,
  "Its sessions are deleted with their chats and their children's worktrees and branches.",
  () => post(store, 'workspace', {action: 'delete', id: workspace.id}));

// chooseSession shows a session, starting it if it is stopped or failed to start.
export function chooseSession(store, id) {
  store.update(state => selectSession(state, id));
  const session = allSessions(store.get()).find(s => s.id === id);
  if (session && !session.ended && (!session.running || failed(session))) startSession(store, session.parent || id);
}

// createSession sends the unsent session's first prompt, which creates the
// session, then shows it. A refusal restores the prompt only while that
// unsent session is still open.
export async function createSession(store, prompt) {
  store.update(state => unsentSending(state, true));
  const unsent = store.get().unsent;
  const {workspace, profile, worktree} = unsent;
  const params = {action: 'new', workspace, profile, ...(worktree && {worktree: '1'})};
  const created = await post(store, 'session', params, JSON.stringify(prompt));
  // A completion must not change another unsent session or restore into it.
  if (store.get().unsent?.identity !== unsent.identity) return true;
  store.update(state => unsentSending(state, false));
  if (created === null) return false;
  const ids = JSON.parse(created);
  store.update(state => unsentCreated(state, ids.session));
  chooseSession(store, ids.session);
  return true;
}

// requestMerge asks a worktree session's agent to merge its branch into the
// checkout's branch.
export const requestMerge = (store, session, target, path) => post(store, 'session', {action: 'input', id: session.id},
  JSON.stringify({text: `Commit any remaining work, then merge your branch into ${target} in the checkout at ${path}, resolving any conflicts, and report the result.`, images: []}));

// sendPrompt sends a session's draft. The unsent session's first prompt
// creates the session. A refused prompt returns to its draft.
export async function sendPrompt(store, session) {
  const state = store.get();
  if (!canSend(state, session)) return;
  const {text, images} = draftOf(state, session.id);
  store.update(next => cleared(next, session.id));
  const accepted = session.id === unsentKey
    ? await createSession(store, {text, images})
    : await post(store, 'session', {action: 'input', id: session.id}, JSON.stringify({text, images})) !== null;
  if (!accepted) store.update(next => restored(next, session.id, {text, images}));
}
