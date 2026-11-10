import {afterEach, expect, test} from 'bun:test';
import {clickControl, find, findAll, mount, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {AllPullRequests} from './all-pull-requests.tsx';
import type {GitHubAccount, PullRequest} from './types.ts';

afterEach(unmountAll);

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
  expect(rows(root, 'Your pull requests to fix').map(row => row.textContent)).toEqual(['#4 ConflictedHas merge conflicts.In o/r for kkestell.Clone']);
  expect(rows(root, 'Ready to merge').map(row => row.textContent)).toEqual(['#1 Add itIn o/r for kkestell.Clone']);
  expect(rows(root, 'Reviews to do').map(row => row.textContent)).toEqual([
    '#2 TheirsFrom bob.In o/r for kkestell.ReviewClone',
    '#3 Other repoFrom bob.In x/y for work.ReviewClone',
  ]);
  expect(rows(root, 'Waiting on others').map(row => row.textContent)).toEqual(['#5 BlockedBlocked from merging.In o/r for kkestell.Clone']);
});

test('Review is disabled and Clone shown only where no workspace tracks the repository', () => {
  const github = {repo: 'O/R', url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 1};
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', github})], accounts: [
    account('a', [pr({number: 1, repo: 'o/r', url: 'u1'}), pr({number: 2, repo: 'x/y', url: 'u2'})]),
  ]}));
  const root = mount(store, <AllPullRequests />);
  expect(labels(root)).toEqual(['Review', 'Review', 'Clone x/y']);
  const review = findAll(root, 'li button').filter(button => button.textContent === 'Review') as HTMLButtonElement[];
  expect(review.map(button => button.disabled)).toEqual([false, true]);
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
  expect(rows(root, 'Reviews to do').map(row => row.querySelector('p:last-of-type')?.textContent)).toEqual(['In o/r for a.', 'In o/r for b.']);
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
