// Reviewing a pull request: the server starts a worktree session at its head
// commit and sends it the review prompt; the page then shows that session.
import {submitPrompt} from '../api.ts';
import {showError} from '../notifications/errors.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import type {Store} from '../store.ts';
import type {PullRequest} from './types.ts';

// canReview is whether someone asked the account itself to review the pull
// request.
export const canReview = (pr: PullRequest) => pr.requested === 'user';

// startReview starts the review session in the workspace and shows it. An
// error is shown; a session created before the error is still shown.
export async function startReview(store: Store, workspaceId: string, number: number) {
  const {session, error} = await submitPrompt({action: 'review', workspace: workspaceId, number: String(number)}, '');
  if (session) chooseSession(store, session);
  if (error) showError(store, error);
}
