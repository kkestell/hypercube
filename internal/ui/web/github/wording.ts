// The sentences and icon looks that describe a branch's tracking and a pull
// request.
import type {IconName} from '../shared/icon.tsx';
import type {GitHubStatus, PullRequest} from './types.ts';

export const commits = (count: number) => `${count} ${count === 1 ? 'commit' : 'commits'}`;

export function trackingText({branch, upstream, ahead, behind}: GitHubStatus): string {
  if (!branch) return 'No branch is checked out.';
  if (!upstream) return `${branch} has no upstream branch.`;
  if (!ahead && !behind) return `${branch} is up to date with ${upstream}.`;
  const position = ahead && behind
    ? `${commits(ahead)} ahead of and ${commits(behind)} behind`
    : ahead ? `${commits(ahead)} ahead of` : `${commits(behind)} behind`;
  return `${branch} is ${position} ${upstream}.`;
}

const sentence = (text: string) => text.endsWith('.') ? text : text + '.';

const time = (ms: number) => new Date(ms).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});

// failureText says what failed, why in the error's first line, and when the
// data still shown was read.
export const failureText = (what: string, error: string, updated: number) =>
  `${what}: ${sentence(error.split('\n')[0])}${updated > 0 ? ` Showing data from ${time(updated)}.` : ''}`;

const failing = (pr: PullRequest) => pr.checks === 'FAILURE' || pr.checks === 'ERROR';
const running = (pr: PullRequest) => pr.checks === 'PENDING' || pr.checks === 'EXPECTED';

// pullRequestFacts is the pull request's draft, review, and check state as
// sentences, in that order.
export function pullRequestFacts(pr: PullRequest): string[] {
  return [
    pr.draft && 'Draft.',
    {APPROVED: 'Approved.', CHANGES_REQUESTED: 'Changes requested.', REVIEW_REQUIRED: 'Review required.'}[pr.review],
    pr.checks === 'SUCCESS' && 'Checks passed.',
    failing(pr) && 'Checks failing.',
    running(pr) && 'Checks running.',
  ].filter((fact): fact is string => !!fact);
}

// pullRequestLook chooses the icon and color that say what a pull request
// needs: draft, failing checks, changes, or nothing (approved).
export function pullRequestLook(pr: PullRequest): {icon: IconName; color: string} {
  const muted = {icon: 'git-pull-request' as const, color: 'text-muted-foreground'};
  if (pr.draft) return {icon: 'git-pull-request-draft', color: 'text-muted-foreground'};
  if (failing(pr)) return {...muted, color: 'text-destructive'};
  if (pr.review === 'CHANGES_REQUESTED') return {...muted, color: 'text-amber-600 dark:text-amber-500'};
  if (pr.review === 'APPROVED' && (pr.checks === 'SUCCESS' || !pr.checks)) return {...muted, color: 'text-green-600 dark:text-green-500'};
  return muted;
}
