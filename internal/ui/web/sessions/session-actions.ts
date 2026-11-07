// What the user does to sessions and workspaces, and the prompts they send.
// The server applies each action, and the result arrives in the next state
// message.
import {post, submitPrompt} from '../api.ts';
import {hypercube} from '../app/window.ts';
import {canSend, cleared, draftOf, isUnsent, unsentCreated, unsentOf} from '../agent/drafts.ts';
import type {Image} from '../images.ts';
import {askToConfirm} from '../shared/confirm.ts';
import type {Store} from '../store.ts';
import {inWorktree, openUnsent, selectSession, unsentSending} from './selection.ts';
import type {Session, Workspace} from './types.ts';
import {addedSubmission, movedSubmissions, submissionsOf, updatedSubmission, withSubmissions, type Submission} from './submissions.ts';

export const archiveSession = (store: Store, id: string) => post(store, 'session', {action: 'archive', id});
export const unarchiveSession = (store: Store, id: string) => post(store, 'session', {action: 'unarchive', id});
export const restartSession = (store: Store, id: string) => post(store, 'session', {action: 'restart', id});
export const renameSession = (store: Store, id: string, title: string) => post(store, 'session', {action: 'rename', id, title});
export const markSeen = (store: Store, id: string) => post(store, 'session', {action: 'seen', id});
// addWorkspace asks the desktop app for a folder and opens a new session in the
// workspace added there.
export async function addWorkspace(store: Store) {
  const path = await hypercube()?.chooseFolder();
  if (!path) return;
  const workspace = await post(store, 'workspace', {action: 'new', path});
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

// createSession is also used by settings flows that submit a first prompt.
export async function createSession(store: Store, prompt: Prompt, key: string) {
  if (!unsentOf(store.get(), key)?.settings) return false;
  const submission = newSubmission(prompt);
  store.update(state => addedSubmission(cleared(state, key), key, submission));
  return deliverSubmission(store, key, submission);
}

const newSubmission = (prompt: Prompt): Submission => ({
  text: prompt.text, images: prompt.images, id: 'submission-' + crypto.randomUUID(), status: 'sending',
});

async function deliverSubmission(store: Store, key: string, submission: Submission): Promise<boolean> {
  const unsent = isUnsent(key) ? unsentOf(store.get(), key) : undefined;
  if (isUnsent(key) && (!unsent?.workspace || !unsent.settings)) return false;
  if (unsent) store.update(state => unsentSending(state, true, key));
  const params: Record<string, string> = unsent
    ? {action: 'new', workspace: unsent.workspace!, ...unsent.settings!, ...(inWorktree(store.get(), unsent) && {worktree: '1'})}
    : {action: 'input', id: key};
  const result = await submitPrompt(params, JSON.stringify({text: submission.text, images: submission.images, submission: submission.id}));
  store.update(state => {
    // A removed workspace or replaced unfinished session no longer owns this reply.
    if (unsent && unsentOf(state, key)?.identity !== unsent.identity) return state;
    let next = unsent ? unsentSending(state, false, key) : state;
    let target = key;
    if (unsent && result.session) {
      const shown = next.unsent?.identity === unsent.identity;
      next = unsentCreated(movedSubmissions(next, key, result.session), result.session, key);
      target = result.session;
      if (shown) next = selectSession(next, result.session);
    }
    return updatedSubmission(next, target, submission.id, result.error
      ? {status: 'failed', error: result.error}
      : {status: 'sent', error: undefined});
  });
  return !result.error;
}

// Retry owns the retained message, leaving any newer composer draft alone.
// A fresh ID distinguishes this attempt; the server does not promise deduplication.
export async function retrySubmission(store: Store, key: string, id: string) {
  const previous = submissionsOf(store.get(), key).find(entry => entry.id === id);
  if (previous?.status !== 'failed') return;
  if (isUnsent(key) && unsentOf(store.get(), key)?.sending) return;
  const submission = newSubmission(previous);
  store.update(state => withSubmissions(state, key, submissionsOf(state, key).map(entry => entry.id === id ? submission : entry)));
  await deliverSubmission(store, key, submission);
}

export const dismissSubmission = (store: Store, key: string, id: string) => store.update(state =>
  withSubmissions(state, key, submissionsOf(state, key).filter(entry => entry.id !== id)));

// requestMerge asks a worktree session's agent to merge its branch into the
// checkout's branch.
export const requestMerge = (store: Store, session: Pick<Session, 'id'>, target: string, path: string) => post(store, 'session', {action: 'input', id: session.id},
  JSON.stringify({text: `Commit any remaining work, then merge your branch into ${target} in the checkout at ${path}, resolving any conflicts, and report the result.`, images: []}));

// Clear the composer and show its complete prompt in the same update, before
// waiting for HTTP or the agent. Each failure keeps its own retryable message.
export async function sendPrompt(store: Store, session: Pick<Session, 'id' | 'ended' | 'activity'>) {
  const state = store.get();
  if (!canSend(state, session)) return;
  const {text, images} = draftOf(state, session.id);
  const submission = newSubmission({text, images});
  store.update(next => addedSubmission(cleared(next, session.id), session.id, submission));
  await deliverSubmission(store, session.id, submission);
}
