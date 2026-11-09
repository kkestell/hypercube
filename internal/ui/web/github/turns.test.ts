import {expect, test} from 'bun:test';
import {testPullRequest} from '../../test-state.ts';
import {sortTurns, turnOf} from './turns.ts';
import type {PullRequest} from './types.ts';

const own = (overrides: Partial<PullRequest>) => testPullRequest({author: 'me', ...overrides});
const theirs = (overrides: Partial<PullRequest>) => testPullRequest({author: 'bob', ...overrides});

test('each pull request is the account’s move, a wait, or none of its business', () => {
  const cases: [PullRequest, string | null, boolean?][] = [
    [theirs({requested: 'user'}), 'Review requested.', true],
    [theirs({reviewed: 'CHANGES_REQUESTED', responded: true}), 'Updated since you requested changes.', true],
    [theirs({unanswered: 2}), '2 replies to your comments.', true],
    [theirs({requested: 'team'}), 'Review requested from your team.', true],
    [theirs({reviewed: 'CHANGES_REQUESTED'}), 'You requested changes.', false],
    [theirs({reviewed: 'APPROVED', responded: true}), null],
    [theirs({}), null],
    [own({draft: true, requiredFailed: true}), 'Draft.', false],
    [own({requiredFailed: true, merge: 'DIRTY'}), 'A required check failed.', true],
    [own({checks: 'FAILURE', merge: 'UNSTABLE'}), 'Ready to merge.', true],
    [own({merge: 'DIRTY', review: 'CHANGES_REQUESTED'}), 'Has merge conflicts.', true],
    [own({merge: 'BEHIND'}), 'Behind the base branch.', true],
    [own({review: 'CHANGES_REQUESTED', merge: 'BLOCKED'}), 'Changes requested.', true],
    [own({review: 'CHANGES_REQUESTED', responded: true, merge: 'BLOCKED'}), 'Waiting for another review.', false],
    [own({unanswered: 1, merge: 'CLEAN'}), '1 unanswered comment.', true],
    [own({merge: 'CLEAN'}), 'Ready to merge.', true],
    [own({checks: 'PENDING', review: 'REVIEW_REQUIRED', merge: 'BLOCKED'}), 'Checks running.', false],
    [own({review: 'REVIEW_REQUIRED', merge: 'BLOCKED'}), 'Waiting for review.', false],
    [own({merge: 'BLOCKED'}), 'Blocked from merging.', false],
    [own({merge: 'UNKNOWN'}), 'Waiting on GitHub.', false],
  ];
  for (const [pr, reason, yours] of cases) {
    const turn = turnOf(pr, 'me');
    expect(turn && {reason: turn.reason, yours: turn.yours}).toEqual(reason === null ? null : {reason, yours: yours!});
  }
});

test('others blocked on the account come first, then its own work, then team requests', () => {
  const prs = [
    theirs({number: 1, requested: 'team'}),
    own({number: 2, merge: 'CLEAN'}),
    own({number: 3, merge: 'DIRTY'}),
    own({number: 4, review: 'REVIEW_REQUIRED'}),
    theirs({number: 5, unanswered: 1}),
    theirs({number: 6}),
    theirs({number: 7, requested: 'user'}),
    own({number: 8, merge: 'BEHIND'}),
  ];
  const {mine, waiting} = sortTurns(prs, 'me');
  expect(mine.map(({pr}) => pr.number)).toEqual([7, 5, 3, 8, 2, 1]);
  expect(waiting.map(({pr}) => pr.number)).toEqual([4]);
});
