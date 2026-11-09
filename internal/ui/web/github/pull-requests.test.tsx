import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testSession, testState, testWorkspace} from '../../test-state.ts';
import {draftOf, unsentKey} from '../agent/drafts.ts';
import {selectWorkspace} from '../sessions/selection.ts';
import {WorkspaceDashboard} from '../sessions/workspace-dashboard.tsx';
import {createStore} from '../store.ts';
import {instruction} from './instructions.ts';
import type {GitHubStatus, PullRequest} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const pr = (overrides: Partial<PullRequest> = {}) => testPullRequest({author: 'kkestell', ...overrides});

const github = (overrides: Partial<GitHubStatus> = {}): GitHubStatus => ({
  repo: 'o/r', url: 'https://github.com/o/r', account: 'kkestell', branch: 'main', upstream: 'origin/main',
  ahead: 0, behind: 0, pullRequests: [], open: 0, updated: 1, ...overrides,
});

function dashboard(status: GitHubStatus, sessions = [testSession({id: 'a'})]) {
  const store = createStore(selectWorkspace(testState({workspaces: [testWorkspace({id: 'w', name: 'repo', github: status, sessions})]}), 'w'));
  return {store, root: mount(store, <WorkspaceDashboard />)};
}

const list = (root: HTMLElement, label: string) => find(root, `[aria-label="${label}"]`);
const buttons = (root: HTMLElement) => findAll(root, '[aria-label="Your move"] button, [aria-label="Waiting on others"] button');

test('rows say why they are the account’s move or a wait, and link GitHub', () => {
  const {root} = dashboard(github({open: 3, pullRequests: [
    pr({number: 7, merge: 'CLEAN'}),
    pr({number: 8, author: 'bob', title: 'Theirs', url: 'https://github.com/o/r/pull/8', requested: 'user'}),
    pr({number: 9, title: 'Mine', review: 'REVIEW_REQUIRED', merge: 'BLOCKED'}),
  ]}));
  const link = find(list(root, 'Your move'), 'a');
  expect(link.textContent).toBe('#8 Theirs');
  expect(link.getAttribute('href')).toBe('https://github.com/o/r/pull/8');
  expect(link.getAttribute('target')).toBe('_blank');
  expect(list(root, 'Your move').textContent).toBe('#8 TheirsReview requested.From bob.#7 Add itReady to merge.Merge');
  expect(list(root, 'Waiting on others').textContent).toBe('#9 MineWaiting for review.');
  expect(buttons(root).map(b => b.textContent)).toEqual(['Merge']);
  expect(root.textContent).not.toContain('other open');
});

test('empty states say what is happening', () => {
  const states: [Partial<GitHubStatus>, string][] = [
    [{account: ''}, 'Choose an account to see pull requests.'],
    [{updated: 0}, 'Checking GitHub.'],
    [{updated: 0, error: 'no network'}, 'Could not load pull requests.'],
    [{}, 'No open pull requests.'],
    [{open: 1}, 'Nothing needs you.'],
  ];
  for (const [overrides, expected] of states) {
    const {root} = dashboard(github(overrides));
    expect(find(root, 'section section').textContent).toContain(expected);
    expect(root.querySelector('[aria-label="Your move"]')).toBeNull();
    unmountAll();
  }
});

test('open pull requests that do not concern the account link to GitHub', () => {
  const {root} = dashboard(github({pullRequests: [pr({merge: 'CLEAN'}), pr({number: 8, author: 'bob'})], open: 61}));
  const link = findAll(root, 'a').find(a => a.textContent === '60 other open pull requests');
  expect(link?.getAttribute('href')).toBe('https://github.com/o/r/pulls');
});

test('Fix checks lists new session and live sessions, matching ones first', () => {
  const failing = pr({checks: 'FAILURE', requiredFailed: true});
  const {root} = dashboard(github({pullRequests: [failing], open: 1}), [
    testSession({id: 'a', title: 'Other'}),
    testSession({id: 'b', title: 'Done', ended: true}),
    testSession({id: 'c', title: 'On it', pullRequest: failing}),
  ]);
  clickControl(buttons(root).find(b => b.textContent === 'Fix checks')!);
  const items = findAll(document.body, '[cmdk-item]').map(item => item.textContent);
  expect(items).toEqual(['New Session', 'On it', 'Other']);
});

test('choosing a session sends the instruction and shows it', async () => {
  const failing = pr({checks: 'FAILURE', requiredFailed: true});
  const stub = stubFetch(() => new Response(''));
  const {root, store} = dashboard(github({pullRequests: [failing], open: 1}), [testSession({id: 'a', title: 'Other'})]);
  clickControl(buttons(root).find(b => b.textContent === 'Fix checks')!);
  await act(async () => clickControl(findAll(document.body, '[cmdk-item]').find(item => item.textContent === 'Other')!));
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'input', id: 'a'});
  expect(JSON.parse(String(stub.mock.calls[0][1]?.body)).text).toBe(instruction('checks', failing));
  expect(store.get().selected).toBe('a');
});

test('choosing New Session opens the unsent session with the instruction', async () => {
  const failing = pr({checks: 'FAILURE', requiredFailed: true});
  const {root, store} = dashboard(github({pullRequests: [failing], open: 1}));
  clickControl(buttons(root).find(b => b.textContent === 'Fix checks')!);
  await act(async () => clickControl(find(document.body, '[cmdk-item]')));
  expect(store.get().unsent?.workspace).toBe('w');
  expect(draftOf(store.get(), unsentKey('w')).text).toBe(instruction('checks', failing));
});
