// Chatting about a pull request: the server fetches its head commit into the
// workspace, and the page opens the workspace's new session set to a new
// worktree on the pull request's branch at that commit. The user writes and
// sends the first prompt.
import {post} from '../api.ts';
import {openUnsent, unsentWorktree} from '../sessions/selection.ts';
import type {NewBranch} from '../sessions/types.ts';
import type {Store} from '../store.ts';
import type {PullRequest} from './types.ts';

// startChat opens the workspace's new session on the pull request's branch,
// keeping what was already typed there. An unsent session that is sending
// keeps its location. A refusal is shown and changes nothing.
export async function startChat(store: Store, workspaceId: string, account: string, pr: PullRequest) {
  const answer = await post(store, 'workspace', {action: 'pull-request-branch', id: workspaceId, account, repo: pr.repo, number: String(pr.number)});
  if (answer === null) return;
  const branch: NewBranch = JSON.parse(answer);
  store.update(s => {
    const next = openUnsent(s, workspaceId);
    return next.unsent?.sending ? next : unsentWorktree(next, true, branch);
  });
}
