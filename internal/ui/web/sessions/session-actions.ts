// What the user does to sessions and workspaces, and the prompts they send.
// The server applies each action, and the result arrives in the next state
// message.
import {post} from '../api.ts';
import {canSend, cleared, draftOf, isUnsent, restored, unsentCreated, unsentOf} from '../agent/drafts.ts';
import type {Image} from '../images.ts';
import {askToConfirm} from '../shared/confirm.ts';
import type {Store} from '../store.ts';
import {inWorktree, openUnsent, selectSession, unsentSending} from './selection.ts';
import type {Session, Workspace} from './types.ts';

export const archiveSession = (store: Store, id: string) => post(store, 'session', {action: 'archive', id});
export const unarchiveSession = (store: Store, id: string) => post(store, 'session', {action: 'unarchive', id});
export const restartSession = (store: Store, id: string) => post(store, 'session', {action: 'restart', id});
export const renameSession = (store: Store, id: string, title: string) => post(store, 'session', {action: 'rename', id, title});
export const markSeen = (store: Store, id: string) => post(store, 'session', {action: 'seen', id});
// addWorkspace opens a new session in the added workspace.
export async function addWorkspace(store: Store) {
  const workspace = await post(store, 'workspace', {action: 'new'});
  if (workspace) store.update(s => openUnsent(s, workspace));
}

const confirmDelete = (store: Store, name: string, detail: string, accept: () => void) =>
  askToConfirm(store, {message: `Are you sure you want to delete '${name}'?`, detail, primary: 'Delete', accept});

const deleteDetail = (session: Session) => {
  if (session.ended) return session.parent ? "Its chat is deleted." : "Its chat and its children's chats are deleted.";
  if (session.worktree) {
    const branch = session.merge ? ` and branch ${session.merge.branch}` : '';
    return `Its agent stops, and its chat, its worktree ${session.dir}${branch}, and its children and their worktrees and branches are deleted.`;
  }
  return "Its agent stops, and its chat, children, and their worktrees and branches are deleted.";
};

export const deleteSession = (store: Store, session: Session) => confirmDelete(store, session.title, deleteDetail(session),
  () => post(store, 'session', {action: 'delete', id: session.id}));

export const deleteWorkspace = (store: Store, workspace: Workspace) => confirmDelete(store, workspace.name,
  "Its sessions are deleted with their chats and their children's worktrees and branches.",
  () => post(store, 'workspace', {action: 'delete', id: workspace.id}));

// chooseSession shows a session. Showing it starts its agent.
export function chooseSession(store: Store, id: string) {
  store.update(state => selectSession(state, id));
}

interface Prompt {
  text: string;
  images: Image[];
}

// createSession sends the unsent session's first prompt, which creates the
// session, then shows it. A refusal restores the prompt only while that
// unsent session is still open.
export async function createSession(store: Store, prompt: Prompt, key: string) {
  const unsent = unsentOf(store.get(), key);
  if (!unsent?.workspace || !unsent.profile) return false;
  store.update(state => unsentSending(state, true, key));
  const {workspace, profile} = unsent;
  const params = {action: 'new', workspace, profile, ...(inWorktree(store.get(), unsent) && {worktree: '1'})};
  const created = await post(store, 'session', params, JSON.stringify(prompt));
  // A completion must not change another unsent session or restore into it.
  if (unsentOf(store.get(), key)?.identity !== unsent.identity) return true;
  store.update(state => unsentSending(state, false, key));
  if (created === null) return false;
  const ids: {session: string} = JSON.parse(created);
  const shown = store.get().unsent?.identity === unsent.identity;
  store.update(state => unsentCreated(state, ids.session, key));
  if (shown) chooseSession(store, ids.session);
  return true;
}

// requestMerge asks a worktree session's agent to merge its branch into the
// checkout's branch.
export const requestMerge = (store: Store, session: Pick<Session, 'id'>, target: string, path: string) => post(store, 'session', {action: 'input', id: session.id},
  JSON.stringify({text: `Commit any remaining work, then merge your branch into ${target} in the checkout at ${path}, resolving any conflicts, and report the result.`, images: []}));

// sendPrompt sends a session's draft. The unsent session's first prompt
// creates the session. A refused prompt returns to its draft.
export async function sendPrompt(store: Store, session: Pick<Session, 'id' | 'ended' | 'activity'>) {
  const state = store.get();
  if (!canSend(state, session)) return;
  const {text, images} = draftOf(state, session.id);
  store.update(next => cleared(next, session.id));
  const accepted = isUnsent(session.id)
    ? await createSession(store, {text, images}, session.id)
    : await post(store, 'session', {action: 'input', id: session.id}, JSON.stringify({text, images})) !== null;
  if (!accepted) store.update(next => restored(next, session.id, {text, images}));
}
