import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {AllPullRequests} from './all-pull-requests.tsx';
import type {GitHubAccount, PullRequest} from './types.ts';

afterEach(() => { unmountAll(); mock.restore(); });

const pr = (overrides: Partial<PullRequest>) => testPullRequest({author: 'bob', requested: 'user', ...overrides});
const account = (login: string, pullRequests: PullRequest[], overrides: Partial<GitHubAccount> = {}): GitHubAccount =>
  ({login, pullRequests, updated: 1, ...overrides});

function view(accounts: GitHubAccount[]) {
  return mount(createStore(testState({workspaces: [testWorkspace({id: 'w'})], accounts})), <AllPullRequests />);
}

const rows = (root: HTMLElement, group: string) => findAll(root, `[aria-label="${group}"] li`);
const labels = (root: HTMLElement) => findAll(root, 'li button').map(button => button.getAttribute('aria-label') ?? button.textContent);
const headings = (root: HTMLElement) => findAll(root, '[aria-expanded]').map(button => button.textContent);

test('groups list their counts, own work before reviews, and rows say where they are', () => {
  const root = view([
    account('kkestell', [
      testPullRequest({number: 1, url: 'https://github.com/o/r/pull/1', repo: 'o/r', author: 'kkestell', merge: 'CLEAN'}),
      pr({number: 2, url: 'https://github.com/o/r/pull/2', repo: 'o/r', title: 'Theirs'}),
      testPullRequest({number: 4, url: 'https://github.com/o/r/pull/4', repo: 'o/r', author: 'kkestell', merge: 'DIRTY', title: 'Conflicted'}),
      testPullRequest({number: 5, url: 'https://github.com/o/r/pull/5', repo: 'o/r', author: 'kkestell', merge: 'BLOCKED', title: 'Blocked'}),
    ]),
    account('work', [pr({number: 3, url: 'https://github.com/x/y/pull/3', repo: 'x/y', title: 'Other repo'})]),
  ]);
  expect(headings(root)).toEqual(['Your pull requests to fix1', 'Ready to merge1', 'Reviews to do2', 'Waiting on others1']);
  expect(rows(root, 'Your pull requests to fix').map(row => row.textContent)).toEqual(['#4 ConflictedHas merge conflicts.o/rChat']);
  expect(rows(root, 'Ready to merge').map(row => row.textContent)).toEqual(['#1 Add ito/rChat']);
  expect(rows(root, 'Reviews to do').map(row => row.textContent)).toEqual([
    '#2 TheirsFrom bob.o/rReviewChat',
    '#3 Other repoFrom bob.x/yReviewChat',
  ]);
  expect(rows(root, 'Waiting on others').map(row => row.textContent)).toEqual(['#5 BlockedBlocked from merging.o/rChat']);
});

const github = {repo: 'O/R', url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 1};
const tracked = () => createStore(testState({workspaces: [testWorkspace({id: 'w', github})], accounts: [
  account('a', [pr({number: 1, repo: 'o/r', url: 'u1'}), pr({number: 2, repo: 'x/y', url: 'u2'})]),
]}));
const button = (root: ParentNode, label: string, index = 0) =>
  findAll(root, 'button').filter(b => b.textContent === label)[index] as HTMLButtonElement;
const flush = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });

test('every row has Chat; Review is disabled where no workspace tracks the repository', () => {
  const root = mount(tracked(), <AllPullRequests />);
  expect(labels(root)).toEqual(['Review', 'Chat', 'Review', 'Chat']);
  expect([button(root, 'Review', 0).disabled, button(root, 'Review', 1).disabled]).toEqual([false, true]);
  expect([button(root, 'Chat', 0).disabled, button(root, 'Chat', 1).disabled]).toEqual([false, false]);
});

test('Chat on a tracked row asks for the branch in that workspace without a dialog', async () => {
  const stub = stubFetch(() => Response.json({name: 'feature', start: 'abc123'}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  await act(async () => clickControl(button(root, 'Chat', 0)));
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(Object.fromEntries(new URL(stub.mock.calls[0][0], 'http://x/').searchParams))
    .toEqual({action: 'pull-request-branch', id: 'w', account: 'a', repo: 'o/r', number: '1'});
  expect(store.get().unsent).toMatchObject({workspace: 'w', worktree: true, branch: {name: 'feature', start: 'abc123'}});
});

test('Chat on an untracked row adds the workspace, then opens its new session on the branch', async () => {
  const stub = stubFetch(url => url.includes('action=clone') ? new Response('new-ws') : Response.json({name: 'feature', start: 'abc123'}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  clickControl(button(root, 'Chat', 1));
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add x/y');
  clickControl(button(document, 'Clone'));
  await flush();
  expect(Object.fromEntries(new URL(stub.mock.calls[1][0], 'http://x/').searchParams))
    .toEqual({action: 'pull-request-branch', id: 'new-ws', account: 'a', repo: 'x/y', number: '2'});
  expect(store.get().unsent).toMatchObject({workspace: 'new-ws', worktree: true, branch: {name: 'feature', start: 'abc123'}});
});

test('collapsing a group hides its rows and is remembered', () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w'})], accounts: [account('a', [pr({repo: 'o/r'})])]}));
  const root = mount(store, <AllPullRequests />);
  clickControl(findAll(root, '[aria-expanded]')[0]);
  expect(store.get().workbench.collapsedPullRequestGroups).toEqual({review: true});
  expect(root.querySelector('[aria-label="Reviews to do"]')).toBeNull();
});

test('one pull request under two accounts is two rows', () => {
  const shared = pr({url: 'https://github.com/o/r/pull/9', repo: 'o/r'});
  const root = view([account('a', [shared]), account('b', [shared])]);
  expect(rows(root, 'Reviews to do').map(row => row.querySelector('p:last-of-type')?.textContent)).toEqual(['o/r', 'o/r']);
});

test('with only waits it says nothing needs the account and lists the waits', () => {
  const root = view([account('a', [testPullRequest({author: 'a', merge: 'BLOCKED'})])]);
  expect(root.textContent).toContain('Nothing needs you.');
  expect(rows(root, 'Waiting on others')).toHaveLength(1);
});

test('a failed account warns, with its older data when it has some', () => {
  const root = view([
    account('a', [pr({repo: 'o/r'})], {error: 'GitHub answered 500\nmore'}),
    account('b', [], {updated: 0, error: 'no token for b'}),
  ]);
  expect(root.textContent).toContain('Could not read pull requests for a: GitHub answered 500. Showing data from ');
  expect(root.textContent).toContain('Could not read pull requests for b: no token for b.');
  expect(rows(root, 'Reviews to do')).toHaveLength(1);
});

test('before any account has been read it says GitHub is being checked', () => {
  expect(view([account('a', [], {updated: 0})]).textContent).toContain('Checking GitHub.');
});

test('closing the dialog returns focus to Chat and allows opening it again', async () => {
  const root = view([account('a', [pr({repo: 'o/r'})])]);
  const opener = button(root, 'Chat');
  act(() => opener.focus());
  clickControl(opener);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add o/r');
  await act(async () => {
    clickControl(button(document, 'Cancel'));
    await new Promise(resolve => setTimeout(resolve, 0));
  });
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(document.activeElement).toBe(opener);
  clickControl(opener);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add o/r');
});
