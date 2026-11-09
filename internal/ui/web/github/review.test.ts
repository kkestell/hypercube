import {expect, test} from 'bun:test';
import {testPullRequest} from '../../test-state.ts';
import {canReview} from './review.ts';

test('only a review requested from the account itself can be started', () => {
  expect(canReview(testPullRequest({requested: 'user'}))).toBe(true);
  expect(canReview(testPullRequest({requested: 'team'}))).toBe(false);
  expect(canReview(testPullRequest())).toBe(false);
});
