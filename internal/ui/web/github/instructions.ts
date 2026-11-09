// The review instruction a pull request sends to an agent, and how it is
// delivered: to a chosen session, or as the draft of a new one.
import {post} from '../api.ts';
import {typed, unsentKey} from '../agent/drafts.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import {openUnsent} from '../sessions/selection.ts';
import type {Store} from '../store.ts';
import type {PullRequest} from './types.ts';

// canReview is whether someone asked the account itself to review the pull
// request.
export const canReview = (pr: PullRequest) => pr.requested === 'user';

export function reviewInstruction(pr: PullRequest): string {
  const {number: n, url} = pr;
  return `Review pull request #${n} (${url}): read its description, diff, and discussion with gh, and report what you find. Post nothing to GitHub.`;
}

// sendInstruction gives a session the instruction and shows the session.
export async function sendInstruction(store: Store, sessionId: string, text: string) {
  await post(store, 'session', {action: 'input', id: sessionId}, JSON.stringify({text, images: []}));
  chooseSession(store, sessionId);
}

// startInstruction opens the workspace's new session with the instruction as
// its draft, replacing any unsent text.
export const startInstruction = (store: Store, workspaceId: string, text: string) =>
  store.update(s => typed(openUnsent(s, workspaceId), unsentKey(workspaceId), text));
