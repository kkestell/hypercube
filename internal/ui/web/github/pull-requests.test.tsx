import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testSession, testState, testWorkspace} from '../../test-state.ts';
import {selectWorkspace} from '../sessions/selection.ts';
import {WorkspaceDashboard} from '../sessions/workspace-dashboard.tsx';
import {createStore} from '../store.ts';
import type {GitHubStatus, PullRequest} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const pr = (overrides: Partial<PullRequest> = {}) => testPullRequest({author: 'kkestell', ...overrides});

const github = (overrides: Partial<GitHubStatus> = {}): GitHubStatus => ({
  repo: 'o/r', url: 'https://github.com/o/r', account: 'kkestell', branch: 'main', upstream: 'origin/main',
  ahead: 0, behind: 0, pullRequests: [], updated: 1, ...overrides,
});

function dashboard(status: GitHubStatus, sessions = [testSession({id: 'a'})]) {
  const store = createStore(selectWorkspace(testState({workspaces: [testWorkspace({id: 'w', name: 'repo', github: status, sessions})]}), 'w'));
  return {store, root: mount(store, <WorkspaceDashboard />)};
}

const list = (root: HTMLElement, label: string) => find(root, `[aria-label="${label}"]`);
const buttons = (root: HTMLElement) => findAll(root, '[aria-label="Pull requests"] li button');

test('rows are grouped as on the pull requests page, say why, and link GitHub', () => {
  const {root} = dashboard(github({pullRequests: [
    pr({number: 7, merge: 'CLEAN'}),
    pr({number: 8, author: 'bob', title: 'Theirs', url: 'https://github.com/o/r/pull/8', requested: 'user'}),
    pr({number: 9, title: 'Mine', review: 'REVIEW_REQUIRED', merge: 'BLOCKED'}),
  ]}));
  const link = find(list(root, 'Reviews to do'), 'a');
  expect(link.getAttribute('aria-label')).toBe('Open #8 on GitHub');
  expect(link.getAttribute('href')).toBe('https://github.com/o/r/pull/8');
  expect(link.getAttribute('target')).toBe('_blank');
  expect(list(root, 'Reviews to do').textContent).toBe('#8 TheirsFrom bob.Review');
  expect(list(root, 'Ready to merge').textContent).toBe('#7 Add it');
  expect(list(root, 'Waiting on others').textContent).toBe('#9 MineWaiting for review.');
  expect(buttons(root).map(b => b.textContent)).toEqual(['Review']);
});

test('empty states say what is happening', () => {
  const states: [Partial<GitHubStatus>, string][] = [
    [{account: ''}, 'Choose an account to see pull requests.'],
    [{updated: 0}, 'Checking GitHub.'],
    [{updated: 0, error: 'no network'}, 'Could not load pull requests.'],
  ];
  for (const [overrides, expected] of states) {
    const {root} = dashboard(github(overrides));
    expect(find(root, '[aria-label="Pull requests"]').textContent).toContain(expected);
    expect(root.querySelector('[aria-label="Pull requests"] ul')).toBeNull();
    unmountAll();
  }
});

test('every dashboard links to all open pull requests', () => {
  const {root} = dashboard(github({pullRequests: [pr({merge: 'CLEAN'}), pr({number: 8, author: 'bob'})]}));
  const link = find(root, '[aria-label="All open pull requests on GitHub"]');
  expect(link.getAttribute('href')).toBe('https://github.com/o/r/pulls');
});

test('an empty list says nothing needs the account', () => {
  const {root} = dashboard(github());
  expect(find(root, '[aria-label="Pull requests"]').textContent).toContain('Nothing needs you.');
});

const requested = () => pr({author: 'bob', requested: 'user'});

test('Review starts a review session, disabled until the server answers, then shows it', async () => {
  let answer!: (response: Response) => void;
  const stub = stubFetch(() => new Promise<Response>(resolve => { answer = resolve; }));
  const {root, store} = dashboard(github({pullRequests: [requested()]}));
  const review = () => buttons(root).find(b => b.textContent === 'Review') as HTMLButtonElement;
  act(() => clickControl(review()));
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(request.pathname).toBe('/session');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'review', workspace: 'w', number: '7'});
  expect(stub.mock.calls[0][1]?.method).toBe('POST');
  expect(review().disabled).toBe(true);
  await act(async () => answer(Response.json({session: 'new-session'})));
  expect(store.get().pending).toBe('new-session');
});

test('a review session whose prompt failed is still shown, with the error', async () => {
  stubFetch(() => Response.json({session: 'new-session', error: 'the agent did not start'}, {status: 409}));
  const {root, store} = dashboard(github({pullRequests: [requested()]}));
  await act(async () => clickControl(buttons(root).find(b => b.textContent === 'Review')!));
  expect(store.get().errors.map(error => error.message)).toEqual(['the agent did not start']);
  expect(store.get().pending).toBe('new-session');
});

test('a refused review shows why and keeps the selection', async () => {
  stubFetch(() => new Response('Choose a model in Settings to start a review', {status: 409}));
  const {root, store} = dashboard(github({pullRequests: [requested()]}));
  const {selected, pending} = store.get();
  await act(async () => clickControl(buttons(root).find(b => b.textContent === 'Review')!));
  expect(store.get().errors.map(error => error.message)).toEqual(['Choose a model in Settings to start a review']);
  expect({selected: store.get().selected, pending: store.get().pending}).toEqual({selected, pending});
  expect((buttons(root).find(b => b.textContent === 'Review') as HTMLButtonElement).disabled).toBe(false);
});
