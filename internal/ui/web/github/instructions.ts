// The instructions a pull request's actions send to an agent, and how they are
// delivered: to a chosen session, or as the draft of a new one.
import {post} from '../api.ts';
import {typed, unsentKey} from '../agent/drafts.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import {openUnsent} from '../sessions/selection.ts';
import type {Store} from '../store.ts';
import type {PullRequest} from './types.ts';

export type PullRequestAction = 'merge' | 'review' | 'checks';

export const actionLabels: Record<PullRequestAction, string> = {merge: 'Merge', review: 'Address review', checks: 'Fix checks'};

// actionsFor lists the actions that apply to one of the account's pull
// requests.
export function actionsFor(pr: PullRequest): PullRequestAction[] {
  return [
    !pr.draft && ['CLEAN', 'HAS_HOOKS', 'UNSTABLE'].includes(pr.merge) && 'merge',
    ((pr.review === 'CHANGES_REQUESTED' && !pr.responded) || pr.unanswered > 0) && 'review',
    (pr.checks === 'FAILURE' || pr.checks === 'ERROR') && 'checks',
  ].filter((action): action is PullRequestAction => !!action);
}

export function instruction(action: PullRequestAction, pr: PullRequest): string {
  const {number: n, url, branch} = pr;
  switch (action) {
    case 'merge':
      return `Merge pull request #${n} (${url}). Check that it is approved and its checks pass, use a merge method the repository allows, and report the result.`;
    case 'review':
      return `Address the review of pull request #${n} (${url}): read its review comments with gh, make the requested changes on its branch ${branch}, push them, and report what you changed.`;
    case 'checks':
      return `Fix the failing checks of pull request #${n} (${url}): read the failing checks' logs with gh, fix the cause on its branch ${branch}, push the fix, and report the result.`;
  }
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
