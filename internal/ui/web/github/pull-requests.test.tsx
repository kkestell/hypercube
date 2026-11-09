import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
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

const pr = (overrides: Partial<PullRequest> = {}): PullRequest => ({
  number: 7, title: 'Add it', url: 'https://github.com/o/r/pull/7', author: 'alice', branch: 'feature', fork: false, draft: false, review: '', checks: '', ...overrides,
});

const github = (overrides: Partial<GitHubStatus> = {}): GitHubStatus => ({
  repo: 'o/r', url: 'https://github.com/o/r', account: 'kkestell', branch: 'main', upstream: 'origin/main',
  ahead: 0, behind: 0, pullRequests: [], open: 0, updated: 1, ...overrides,
});

function dashboard(status: GitHubStatus, sessions = [testSession({id: 'a'})]) {
  const store = createStore(selectWorkspace(testState({workspaces: [testWorkspace({id: 'w', name: 'repo', github: status, sessions})]}), 'w'));
  return {store, root: mount(store, <WorkspaceDashboard />)};
}

const text = (root: HTMLElement) => find(root, '[aria-label="Pull requests"]').textContent;

test('a row links the pull request and shows its author and facts', () => {
  const {root} = dashboard(github({pullRequests: [pr({review: 'APPROVED', checks: 'SUCCESS'})], open: 1}));
  const link = find(root, '[aria-label="Pull requests"] a');
  expect(link.textContent).toBe('#7 Add it');
  expect(link.getAttribute('href')).toBe('https://github.com/o/r/pull/7');
  expect(link.getAttribute('target')).toBe('_blank');
  expect(text(root)).toContain('aliceApproved.Checks passed.');
  expect(findAll(root, '[aria-label="Pull requests"] button').map(b => b.textContent)).toEqual(['Merge']);
  expect(root.textContent).not.toContain('View all');
});

test('empty states say what is happening', () => {
  const states: [Partial<GitHubStatus>, string][] = [
    [{account: ''}, 'Choose an account to see pull requests.'],
    [{updated: 0}, 'Checking GitHub.'],
    [{updated: 0, error: 'no network'}, 'Could not load pull requests.'],
    [{}, 'No open pull requests.'],
  ];
  for (const [overrides, expected] of states) {
    const {root} = dashboard(github(overrides));
    expect(find(root, 'section section').textContent).toContain(expected);
    expect(root.querySelector('[aria-label="Pull requests"]')).toBeNull();
    unmountAll();
  }
});

test('more open pull requests than rows link to GitHub', () => {
  const {root} = dashboard(github({pullRequests: [pr()], open: 61}));
  const link = findAll(root, 'a').find(a => a.textContent === 'View all 61 open pull requests on GitHub');
  expect(link?.getAttribute('href')).toBe('https://github.com/o/r/pulls');
});

test('Fix checks lists new session and live sessions, matching ones first', () => {
  const failing = pr({checks: 'FAILURE'});
  const {root} = dashboard(github({pullRequests: [failing], open: 1}), [
    testSession({id: 'a', title: 'Other'}),
    testSession({id: 'b', title: 'Done', ended: true}),
    testSession({id: 'c', title: 'On it', pullRequest: failing}),
  ]);
  clickControl(findAll(root, '[aria-label="Pull requests"] button').find(b => b.textContent === 'Fix checks')!);
  const items = findAll(document.body, '[cmdk-item]').map(item => item.textContent);
  expect(items).toEqual(['New Session', 'On it', 'Other']);
});

test('choosing a session sends the instruction and shows it', async () => {
  const failing = pr({checks: 'FAILURE'});
  const stub = stubFetch(() => new Response(''));
  const {root, store} = dashboard(github({pullRequests: [failing], open: 1}), [testSession({id: 'a', title: 'Other'})]);
  clickControl(findAll(root, '[aria-label="Pull requests"] button').find(b => b.textContent === 'Fix checks')!);
  await act(async () => clickControl(findAll(document.body, '[cmdk-item]').find(item => item.textContent === 'Other')!));
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'input', id: 'a'});
  expect(JSON.parse(String(stub.mock.calls[0][1]?.body)).text).toBe(instruction('checks', failing));
  expect(store.get().selected).toBe('a');
});

test('choosing New Session opens the unsent session with the instruction', async () => {
  const failing = pr({checks: 'FAILURE'});
  const {root, store} = dashboard(github({pullRequests: [failing], open: 1}));
  clickControl(findAll(root, '[aria-label="Pull requests"] button').find(b => b.textContent === 'Fix checks')!);
  await act(async () => clickControl(find(document.body, '[cmdk-item]')));
  expect(store.get().unsent?.workspace).toBe('w');
  expect(draftOf(store.get(), unsentKey('w')).text).toBe(instruction('checks', failing));
});
