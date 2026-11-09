import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testState, testWorkspace, testPullRequest} from '../../test-state.ts';
import {draftOf, unsentKey} from '../agent/drafts.ts';
import {createStore} from '../store.ts';
import {actionsFor, instruction, sendInstruction, startInstruction} from './instructions.ts';

afterEach(() => mock.restore());

const pr = testPullRequest;

test('actions follow the pull request state', () => {
  expect(actionsFor(pr({draft: true}))).toEqual([]);
  expect(actionsFor(pr({draft: true, review: 'CHANGES_REQUESTED', checks: 'FAILURE'}))).toEqual(['review', 'checks']);
  expect(actionsFor(pr({review: 'CHANGES_REQUESTED', responded: true, checks: 'ERROR', merge: 'UNSTABLE'}))).toEqual(['merge', 'checks']);
  expect(actionsFor(pr({unanswered: 1, merge: 'BLOCKED'}))).toEqual(['review']);
  expect(actionsFor(pr({review: 'APPROVED', checks: 'SUCCESS', merge: 'CLEAN'}))).toEqual(['merge']);
});

test('instructions name the pull request, its link, and its branch', () => {
  const p = pr();
  expect(instruction('merge', p)).toBe('Merge pull request #7 (https://github.com/o/r/pull/7). Check that it is approved and its checks pass, use a merge method the repository allows, and report the result.');
  expect(instruction('review', p)).toBe('Address the review of pull request #7 (https://github.com/o/r/pull/7): read its review comments with gh, make the requested changes on its branch feature, push them, and report what you changed.');
  expect(instruction('checks', p)).toBe("Fix the failing checks of pull request #7 (https://github.com/o/r/pull/7): read the failing checks' logs with gh, fix the cause on its branch feature, push the fix, and report the result.");
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
