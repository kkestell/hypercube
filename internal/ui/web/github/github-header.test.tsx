import {afterEach, expect, mock, test} from 'bun:test';
import {find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {selectWorkspace} from '../sessions/selection.ts';
import {WorkspaceDashboard} from '../sessions/workspace-dashboard.tsx';
import {Sidebar} from '../sidebar/sidebar.tsx';
import {createStore} from '../store.ts';
import {chooseAccount} from './github-header.tsx';
import type {GitHubStatus, PullRequest} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const github = (overrides: Partial<GitHubStatus> = {}): GitHubStatus => ({
  repo: 'kkestell/hypercube', url: 'https://github.com/kkestell/hypercube', account: 'kkestell', branch: 'main', upstream: 'origin/main',
  ahead: 0, behind: 0, pullRequests: [], open: 0, updated: 0, ...overrides,
});

function dashboard(status?: GitHubStatus) {
  const store = createStore(selectWorkspace(testState({workspaces: [testWorkspace({id: 'w', name: 'repo', github: status})]}), 'w'));
  return {store, root: mount(store, <WorkspaceDashboard />)};
}

test('a workspace without GitHub shows only its name', () => {
  const {root} = dashboard();
  expect(root.querySelector('a')).toBeNull();
  expect(root.textContent).not.toContain('GitHub');
});

test('a GitHub workspace shows its repository, account, and tracking', () => {
  const {root} = dashboard(github({ahead: 1}));
  const link = find(root, 'a');
  expect(link.getAttribute('href')).toBe('https://github.com/kkestell/hypercube');
  expect(link.getAttribute('target')).toBe('_blank');
  expect(root.textContent).toContain('Using kkestell.');
  expect(root.textContent).toContain('main is 1 commit ahead of origin/main.');
});

test('several qualifying accounts show a chooser', () => {
  const {root} = dashboard(github({account: '', choices: ['a', 'b']}));
  expect(find(root, '[aria-label="GitHub account"]').textContent).toContain('Choose an account');
  expect(root.textContent).toContain('Agents and terminals in this workspace use this account.');
});

test('choosing an account posts the workspace and account', async () => {
  const stub = stubFetch(() => new Response(''));
  await chooseAccount(createStore(testState()), 'w', 'b');
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(request.pathname).toBe('/workspace');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'github-account', id: 'w', account: 'b'});
});

test('an update error shows its first line and the age of the data', () => {
  const updated = Date.UTC(2026, 10, 9, 12, 30);
  const {root} = dashboard(github({error: 'fetch failed\nmore detail', updated}));
  const age = new Date(updated).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});
  expect(root.textContent).toContain(`Could not update from GitHub: fetch failed. Showing data from ${age}.`);
  expect(root.textContent).not.toContain('more detail');
});

test('an update error before any update shows no age', () => {
  const {root} = dashboard(github({error: 'no network'}));
  expect(root.textContent).toContain('Could not update from GitHub: no network.');
  expect(root.textContent).not.toContain('Showing data from');
});

test('an update error that ends in a period gets no second one', () => {
  const {root} = dashboard(github({error: 'Permission denied (publickey).'}));
  expect(root.textContent).toContain('Could not update from GitHub: Permission denied (publickey).');
  expect(root.textContent).not.toContain('..');
});

test('a session row shows its open pull request', () => {
  stubFetch(() => new Response(''));
  const pullRequest: PullRequest = {number: 12, title: 'Add it', url: '', author: 'a', branch: 'b', fork: false, draft: true, review: '', checks: ''};
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', name: 'repo', sessions: [
    testSession({id: 'a', pullRequest}), testSession({id: 'b'}),
  ]})]}));
  const root = mount(store, <Sidebar searchOpen={false} onSearchOpenChange={() => {}} onSettings={() => {}} />);
  const images = (id: string) => findAll(root, `[data-session="${id}"] [role=img]`).map(image => image.getAttribute('aria-label')).filter(name => name?.startsWith('Pull request'));
  expect(images('a')).toEqual(['Pull request #12: Add it. Draft.']);
  expect(images('b')).toEqual([]);
});
