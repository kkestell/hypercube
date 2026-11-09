import {afterEach, expect, test} from 'bun:test';
import {find, findAll, mount, unmountAll} from '../../test-render.tsx';
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

test('rows from two accounts and repositories say where they are, in rank order', () => {
  const root = view([
    account('kkestell', [
      testPullRequest({number: 1, url: 'https://github.com/o/r/pull/1', repo: 'o/r', author: 'kkestell', merge: 'CLEAN'}),
      pr({number: 2, url: 'https://github.com/o/r/pull/2', repo: 'o/r', title: 'Theirs'}),
    ]),
    account('work', [pr({number: 3, url: 'https://github.com/x/y/pull/3', repo: 'x/y', title: 'Other repo'})]),
  ]);
  const rows = findAll(root, '[aria-label="Your move"] li');
  expect(rows.map(row => row.textContent)).toEqual([
    '#2 TheirsReview requested.From bob.In o/r for kkestell.',
    '#3 Other repoReview requested.From bob.In x/y for work.',
    '#1 Add itReady to merge.In o/r for kkestell.',
  ]);
  expect(find(rows[1], 'p:last-of-type a').getAttribute('href')).toBe('https://github.com/x/y');
  expect(findAll(root, 'button')).toHaveLength(0);
});

test('one pull request under two accounts is two rows', () => {
  const shared = pr({url: 'https://github.com/o/r/pull/9', repo: 'o/r'});
  const root = view([account('a', [shared]), account('b', [shared])]);
  expect(findAll(root, '[aria-label="Your move"] li').map(row => row.textContent?.slice(-13))).toEqual(['In o/r for a.', 'In o/r for b.']);
});

test('pull requests that wait on others are left out', () => {
  const root = view([account('a', [testPullRequest({author: 'a', merge: 'BLOCKED'})])]);
  expect(root.querySelector('[aria-label="Your move"]')).toBeNull();
  expect(root.textContent).toContain('Nothing needs you.');
});

test('a failed account warns, with its older data when it has some', () => {
  const root = view([
    account('a', [pr({repo: 'o/r'})], {error: 'GitHub answered 500\nmore'}),
    account('b', [], {updated: 0, error: 'no token for b'}),
  ]);
  expect(root.textContent).toContain('Could not read pull requests for a: GitHub answered 500. Showing data from ');
  expect(root.textContent).toContain('Could not read pull requests for b: no token for b.');
  expect(findAll(root, '[aria-label="Your move"] li')).toHaveLength(1);
});

test('before any account has been read it says GitHub is being checked', () => {
  expect(view([account('a', [], {updated: 0})]).textContent).toContain('Checking GitHub.');
});
