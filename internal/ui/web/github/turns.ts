// Whose move each pull request is: the account's, someone else's while the
// account waits, or nobody's that concerns the account.
import type {PullRequest} from './types.ts';

// Turn is what a pull request waits on and why. rank orders the account's
// moves: others blocked on the account come first, its own work next.
export interface Turn {
  yours: boolean;
  reason: string;
  rank: number;
}

const plural = (count: number, one: string, many: string) => `${count} ${count === 1 ? one : many}`;
const running = (pr: PullRequest) => pr.checks === 'PENDING' || pr.checks === 'EXPECTED';
const yours = (rank: number, reason: string): Turn => ({yours: true, reason, rank});
const theirs = (reason: string): Turn => ({yours: false, reason, rank: 0});

// turnOf says whose move the pull request is for the account, or null when it
// does not concern the account.
export function turnOf(pr: PullRequest, account: string): Turn | null {
  return pr.author === account ? ownTurn(pr) : reviewTurn(pr);
}

function reviewTurn(pr: PullRequest): Turn | null {
  if (pr.requested === 'user') return yours(0, 'Review requested.');
  if (pr.reviewed === 'CHANGES_REQUESTED' && pr.responded) return yours(1, 'Updated since you requested changes.');
  if (pr.unanswered) return yours(2, `${plural(pr.unanswered, 'reply', 'replies')} to your comments.`);
  if (pr.requested === 'team') return yours(5, 'Review requested from your team.');
  if (pr.reviewed === 'CHANGES_REQUESTED') return theirs('You requested changes.');
  return null;
}

function ownTurn(pr: PullRequest): Turn {
  if (pr.draft) return theirs('Draft.');
  if (pr.requiredFailed) return yours(3, 'A required check failed.');
  if (pr.merge === 'DIRTY') return yours(3, 'Has merge conflicts.');
  if (pr.merge === 'BEHIND') return yours(3, 'Behind the base branch.');
  if (pr.review === 'CHANGES_REQUESTED') return pr.responded ? theirs('Waiting for another review.') : yours(3, 'Changes requested.');
  if (pr.unanswered) return yours(3, `${plural(pr.unanswered, 'unanswered comment', 'unanswered comments')}.`);
  if (['CLEAN', 'HAS_HOOKS', 'UNSTABLE'].includes(pr.merge)) return yours(4, 'Ready to merge.');
  if (running(pr)) return theirs('Checks running.');
  if (pr.review === 'REVIEW_REQUIRED') return theirs('Waiting for review.');
  if (pr.merge === 'BLOCKED') return theirs('Blocked from merging.');
  return theirs('Waiting on GitHub.');
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
