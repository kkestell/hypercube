import {expect, test} from 'bun:test';
import type {IconName} from '../shared/icon.tsx';
import type {GitHubStatus, PullRequest} from './types.ts';
import {pullRequestFacts, pullRequestLook, trackingText} from './wording.ts';

const status = (overrides: Partial<GitHubStatus> = {}): GitHubStatus => ({
  repo: 'o/r', url: 'https://github.com/o/r', account: 'a', branch: 'main', upstream: 'origin/main',
  ahead: 0, behind: 0, pullRequests: [], open: 0, updated: 0, ...overrides,
});
const pr = (overrides: Partial<PullRequest> = {}): PullRequest => ({
  number: 1, title: 't', url: '', author: 'a', branch: 'b', fork: false, draft: false, review: '', checks: '', ...overrides,
});

test('trackingText says where the branch stands', () => {
  const cases: [Partial<GitHubStatus>, string][] = [
    [{branch: ''}, 'No branch is checked out.'],
    [{upstream: ''}, 'main has no upstream branch.'],
    [{}, 'main is up to date with origin/main.'],
    [{ahead: 1}, 'main is 1 commit ahead of origin/main.'],
    [{ahead: 3}, 'main is 3 commits ahead of origin/main.'],
    [{behind: 1}, 'main is 1 commit behind origin/main.'],
    [{behind: 2}, 'main is 2 commits behind origin/main.'],
    [{ahead: 2, behind: 1}, 'main is 2 commits ahead of and 1 commit behind origin/main.'],
  ];
  for (const [overrides, text] of cases) expect(trackingText(status(overrides))).toBe(text);
});

test('pullRequestFacts lists draft, review, then checks', () => {
  const cases: [Partial<PullRequest>, string[]][] = [
    [{}, []],
    [{draft: true, review: 'APPROVED', checks: 'SUCCESS'}, ['Draft.', 'Approved.', 'Checks passed.']],
    [{review: 'CHANGES_REQUESTED', checks: 'FAILURE'}, ['Changes requested.', 'Checks failing.']],
    [{review: 'REVIEW_REQUIRED', checks: 'PENDING'}, ['Review required.', 'Checks running.']],
    [{checks: 'ERROR'}, ['Checks failing.']],
    [{checks: 'EXPECTED'}, ['Checks running.']],
  ];
  for (const [overrides, facts] of cases) expect(pullRequestFacts(pr(overrides))).toEqual(facts);
});

test('pullRequestLook picks the most pressing look', () => {
  const green = 'text-green-600 dark:text-green-500';
  const amber = 'text-amber-600 dark:text-amber-500';
  const cases: [Partial<PullRequest>, IconName, string][] = [
    [{draft: true, checks: 'FAILURE'}, 'git-pull-request-draft', 'text-muted-foreground'],
    [{review: 'CHANGES_REQUESTED', checks: 'FAILURE'}, 'git-pull-request', 'text-destructive'],
    [{checks: 'ERROR'}, 'git-pull-request', 'text-destructive'],
    [{review: 'CHANGES_REQUESTED'}, 'git-pull-request', amber],
    [{review: 'APPROVED', checks: 'SUCCESS'}, 'git-pull-request', green],
    [{review: 'APPROVED'}, 'git-pull-request', green],
    [{review: 'APPROVED', checks: 'PENDING'}, 'git-pull-request', 'text-muted-foreground'],
    [{}, 'git-pull-request', 'text-muted-foreground'],
  ];
  for (const [overrides, icon, color] of cases) expect(pullRequestLook(pr(overrides))).toEqual({icon, color});
});
