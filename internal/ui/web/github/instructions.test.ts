import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testState, testWorkspace, testPullRequest} from '../../test-state.ts';
import {draftOf, unsentKey} from '../agent/drafts.ts';
import {createStore} from '../store.ts';
import {canReview, reviewInstruction, sendInstruction, startInstruction} from './instructions.ts';

afterEach(() => mock.restore());

const pr = testPullRequest;

test('only a review requested from the account itself can be started', () => {
  expect(canReview(pr({requested: 'user'}))).toBe(true);
  expect(canReview(pr({requested: 'team'}))).toBe(false);
  expect(canReview(pr())).toBe(false);
});

test('the review instruction names the pull request and its link', () => {
  expect(reviewInstruction(pr())).toBe('Review pull request #7 (https://github.com/o/r/pull/7): read its description, diff, and discussion with gh, and report what you find. Post nothing to GitHub.');
});

test('sending posts the instruction to the session and shows it', async () => {
  const stub = stubFetch(() => new Response(''));
  const store = createStore(testState());
  await sendInstruction(store, 's1', 'Do it');
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(request.pathname).toBe('/session');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'input', id: 's1'});
  expect(JSON.parse(String(stub.mock.calls[0][1]?.body))).toEqual({text: 'Do it', images: []});
});

test('starting opens the workspace new session with the instruction as its draft', () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w'})]}));
  startInstruction(store, 'w', 'Do it');
  expect(store.get().unsent?.workspace).toBe('w');
  expect(draftOf(store.get(), unsentKey('w')).text).toBe('Do it');
});
