// Whose move each pull request is: the account's, someone else's while the
// account waits, or nobody's that concerns the account.
import type {GitHubAccount, PullRequest} from './types.ts';

export type TurnGroup = 'fix' | 'replies' | 'merge' | 'review' | 'waiting';

// turnGroups are the kinds of work, in the order the pull requests page shows
// them: finishing the account's own work before starting new reviews. A row
// whose reason is its group's implied reason leaves the reason to the heading.
export const turnGroups: {id: TurnGroup; title: string; implied?: string}[] = [
  {id: 'fix', title: 'Your pull requests to fix'},
  {id: 'replies', title: 'Replies and re-reviews'},
  {id: 'merge', title: 'Ready to merge', implied: 'Ready to merge.'},
  {id: 'review', title: 'Reviews to do', implied: 'Review requested.'},
  {id: 'waiting', title: 'Waiting on others'},
];

// Turn is what a pull request waits on and why, and which kind of work it is.
// rank orders the account's moves: others blocked on the account come first,
// its own work next.
export interface Turn {
  yours: boolean;
  reason: string;
  rank: number;
  group: TurnGroup;
}

const plural = (count: number, one: string, many: string) => `${count} ${count === 1 ? one : many}`;
const running = (pr: PullRequest) => pr.checks === 'PENDING' || pr.checks === 'EXPECTED';
const yours = (group: TurnGroup, rank: number, reason: string): Turn => ({yours: true, reason, rank, group});
const theirs = (reason: string): Turn => ({yours: false, reason, rank: 0, group: 'waiting'});

// turnOf says whose move the pull request is for the account, or null when it
// does not concern the account.
export function turnOf(pr: PullRequest, account: string): Turn | null {
  return pr.author === account ? ownTurn(pr) : reviewTurn(pr);
}

function reviewTurn(pr: PullRequest): Turn | null {
  if (pr.requested === 'user') return yours('review', 0, 'Review requested.');
  if (pr.reviewed === 'CHANGES_REQUESTED' && pr.responded) return yours('replies', 1, 'Updated since you requested changes.');
  if (pr.unanswered) return yours('replies', 2, `${plural(pr.unanswered, 'reply', 'replies')} to your comments.`);
  if (pr.requested === 'team' && !pr.reviewed) return yours('review', 5, 'Review requested from your team.');
  if (pr.reviewed === 'CHANGES_REQUESTED') return theirs('You requested changes.');
  return null;
}

function ownTurn(pr: PullRequest): Turn {
  if (pr.draft) return theirs('Draft.');
  if (pr.requiredFailed) return yours('fix', 3, 'A required check failed.');
  if (pr.merge === 'DIRTY') return yours('fix', 3, 'Has merge conflicts.');
  if (pr.merge === 'BEHIND') return yours('fix', 3, 'Behind the base branch.');
  if (pr.review === 'CHANGES_REQUESTED') return pr.responded ? theirs('Waiting for another review.') : yours('fix', 3, 'Changes requested.');
  if (pr.unanswered) return yours('fix', 3, `${plural(pr.unanswered, 'unanswered comment', 'unanswered comments')}.`);
  // Without branch protection GitHub calls a pull request mergeable while
  // requested reviewers have yet to answer.
  if (pr.pendingReviews && pr.review !== 'APPROVED') return theirs('Waiting for review.');
  if (['CLEAN', 'HAS_HOOKS', 'UNSTABLE'].includes(pr.merge)) return yours('merge', 4, 'Ready to merge.');
  if (running(pr)) return theirs('Checks running.');
  if (pr.review === 'REVIEW_REQUIRED') return theirs('Waiting for review.');
  if (pr.merge === 'BLOCKED') return theirs('Blocked from merging.');
  return theirs('Waiting on GitHub.');
}

const allTurns = (accounts: GitHubAccount[], group: 'mine' | 'waiting') =>
  accounts.flatMap(account => sortTurns(account.pullRequests, account.login)[group].map(({pr, turn}) => ({account: account.login, pr, turn})));

// yourMoves lists, for each account in order, its pull requests that are its
// move, with every account's best-ranked first and equal ranks keeping their
// order.
export function yourMoves(accounts: GitHubAccount[]) {
  return allTurns(accounts, 'mine').sort((a, b) => a.turn.rank - b.turn.rank);
}

// othersMoves lists, for each account in order, the pull requests it waits on.
export function othersMoves(accounts: GitHubAccount[]) {
  return allTurns(accounts, 'waiting');
}

// sortTurns splits the pull requests into the account's moves, best first,
// and the ones it waits on, keeping GitHub's most recently updated order
// within a rank.
export function sortTurns(prs: PullRequest[], account: string) {
  const mine: {pr: PullRequest; turn: Turn}[] = [], waiting: {pr: PullRequest; turn: Turn}[] = [];
  for (const pr of prs) {
    const turn = turnOf(pr, account);
    if (turn) (turn.yours ? mine : waiting).push({pr, turn});
  }
  mine.sort((a, b) => a.turn.rank - b.turn.rank);
  return {mine, waiting};
}
