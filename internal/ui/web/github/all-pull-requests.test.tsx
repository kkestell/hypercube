import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {choose, clickControl, find, findAll, menuChoices, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState, testWorkspace} from '../../test-state.ts';
import {draftOf, typed, unsentKey} from '../agent/drafts.ts';
import {openUnsent, unsentSettingsChosen} from '../sessions/selection.ts';
import type {State} from '../state.ts';
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
const headings = (root: HTMLElement) => findAll(root, '[aria-expanded]').map(button => button.textContent);
const row = (root: ParentNode, index = 0) => findAll(root, 'li')[index];
const button = (parent: ParentNode, label: string) => find(parent, `button[aria-label="${label}"]`) as HTMLButtonElement;
const flush = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });

test('groups count their rows and keep the account\'s own work first, with each reason together', () => {
  const root = view([
    account('kkestell', [
      testPullRequest({number: 1, url: 'https://github.com/o/r/pull/1', repo: 'o/r', author: 'kkestell', merge: 'CLEAN'}),
      testPullRequest({number: 6, url: 'https://github.com/o/r/pull/6', repo: 'o/r', author: 'kkestell', review: 'CHANGES_REQUESTED', merge: 'BLOCKED', title: 'Changes'}),
      pr({number: 2, url: 'https://github.com/o/r/pull/2', repo: 'o/r', title: 'Theirs'}),
      testPullRequest({number: 4, url: 'https://github.com/o/r/pull/4', repo: 'o/r', author: 'kkestell', merge: 'DIRTY', title: 'Conflicted'}),
      testPullRequest({number: 7, url: 'https://github.com/o/r/pull/7', repo: 'o/r', author: 'kkestell', review: 'CHANGES_REQUESTED', merge: 'BLOCKED', title: 'More changes'}),
      testPullRequest({number: 5, url: 'https://github.com/o/r/pull/5', repo: 'o/r', author: 'kkestell', merge: 'BLOCKED', title: 'Blocked'}),
    ]),
    account('work', [pr({number: 3, url: 'https://github.com/x/y/pull/3', repo: 'x/y', title: 'Other repo'})]),
  ]);
  expect(headings(root)).toEqual(['To fix3', 'Ready to merge1', 'To review2', 'Waiting on others1']);
  expect(rows(root, 'To fix').map(item => item.getAttribute('title')?.split('\n')[0])).toEqual(['Conflicted', 'Changes', 'More changes']);
  expect(rows(root, 'To review').map(item => item.getAttribute('title'))).toEqual(['Theirs\no/r#2 from bob.', 'Other repo\nx/y#3 from bob.']);
  expect(rows(root, 'To fix')[0].querySelector('[role=img]')?.getAttribute('aria-label')).toBe('Has merge conflicts.');
  expect(rows(root, 'Waiting on others').map(item => item.getAttribute('title'))).toEqual(['Blocked\no/r#5.']);
});

const github = {repo: 'O/R', url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 1};
const tracked = (prepare = (state: State) => state) => createStore(prepare(testState({workspaces: [testWorkspace({id: 'w', github})], accounts: [
  account('a', [pr({number: 7, repo: 'o/r', url: 'u1'}), pr({number: 2, repo: 'x/y', url: 'u2'})]),
]})));

test('a row offers Review and Chat on hover and in its menu, with Review disabled where no workspace tracks the repository', () => {
  const root = mount(tracked(), <AllPullRequests />);
  expect([button(row(root, 0), 'Review').disabled, button(row(root, 1), 'Review').disabled]).toEqual([false, true]);
  expect([button(row(root, 0), 'Chat').disabled, button(row(root, 1), 'Chat').disabled]).toEqual([false, false]);
  expect(menuChoices(row(root, 0))).toEqual([['Review', true], ['Chat', true], ['Open on GitHub', true], ['Copy URL', true]]);
  expect(menuChoices(row(root, 1))).toEqual([['Review', false], ['Chat', true], ['Open on GitHub', true], ['Copy URL', true]]);
  const own = view([account('me', [testPullRequest({author: 'me', merge: 'CLEAN'})])]);
  expect(row(own).querySelector('button[aria-label=Review]')).toBeNull();
});

test('the menu opens the pull request on GitHub and copies its URL', () => {
  const opened = spyOn(window, 'open').mockReturnValue(null);
  const written = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  const root = mount(tracked(), <AllPullRequests />);
  choose(row(root), 'Open on GitHub');
  expect(opened.mock.calls[0]).toEqual(['u1', '_blank', 'noopener']);
  choose(row(root), 'Copy URL');
  expect(written.mock.calls[0]).toEqual(['u1']);
});

test('Shift+F10 on a row opens its menu', () => {
  const root = mount(tracked(), <AllPullRequests />);
  act(() => { row(root).dispatchEvent(new KeyboardEvent('keydown', {key: 'F10', shiftKey: true, bubbles: true})); });
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toContain('Chat');
});

test('Review starts a review session, disabled until the server answers, then shows it', async () => {
  let answer!: (response: Response) => void;
  const stub = stubFetch(() => new Promise<Response>(resolve => { answer = resolve; }));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  act(() => clickControl(button(row(root), 'Review')));
  const request = new URL(stub.mock.calls[0][0], 'http://x/');
  expect(request.pathname).toBe('/session');
  expect(Object.fromEntries(request.searchParams)).toEqual({action: 'review', workspace: 'w', number: '7'});
  expect(stub.mock.calls[0][1]?.method).toBe('POST');
  expect(button(row(root), 'Review').disabled).toBe(true);
  await act(async () => answer(Response.json({session: 'new-session'})));
  expect(store.get().pending).toBe('new-session');
});

test('a review session whose prompt failed is still shown, with the error', async () => {
  stubFetch(() => Response.json({session: 'new-session', error: 'the agent did not start'}, {status: 409}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  await act(async () => clickControl(button(row(root), 'Review')));
  expect(store.get().errors.map(error => error.message)).toEqual(['the agent did not start']);
  expect(store.get().pending).toBe('new-session');
});

test('a refused review shows why and keeps the selection', async () => {
  stubFetch(() => new Response('Choose a model in Settings to start a review', {status: 409}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  const {selected, pending} = store.get();
  await act(async () => clickControl(button(row(root), 'Review')));
  expect(store.get().errors.map(error => error.message)).toEqual(['Choose a model in Settings to start a review']);
  expect({selected: store.get().selected, pending: store.get().pending}).toEqual({selected, pending});
  expect(button(row(root), 'Review').disabled).toBe(false);
});

test('Chat on a tracked row opens the new session on the branch, disabled until the server answers', async () => {
  let answer!: (response: Response) => void;
  const stub = stubFetch(() => new Promise<Response>(resolve => { answer = resolve; }));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  act(() => clickControl(button(row(root), 'Chat')));
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(Object.fromEntries(new URL(stub.mock.calls[0][0], 'http://x/').searchParams))
    .toEqual({action: 'pull-request-branch', id: 'w', account: 'a', repo: 'o/r', number: '7'});
  expect(button(row(root), 'Chat').disabled).toBe(true);
  await act(async () => answer(Response.json({name: 'feature', start: 'abc123'})));
  expect(store.get().unsent).toMatchObject({workspace: 'w', worktree: true, branch: {name: 'feature', start: 'abc123'}});
});

test('Chat keeps what was already typed in the new session and its model', async () => {
  stubFetch(() => Response.json({name: 'feature', start: 'abc123'}));
  const settings = {harness: 'claude', model: 'opus', effort: 'high'};
  const store = tracked(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), settings), unsentKey('w'), 'unfinished'));
  const root = mount(store, <AllPullRequests />);
  await act(async () => clickControl(button(row(root), 'Chat')));
  expect(draftOf(store.get(), unsentKey('w')).text).toBe('unfinished');
  expect(store.get().unsent?.settings).toEqual(settings);
});

test('a refused chat shows why and keeps the selection', async () => {
  stubFetch(() => new Response('pull request #7 in o/r is not open for a', {status: 409}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  const {selected} = store.get();
  await act(async () => clickControl(button(row(root), 'Chat')));
  expect(store.get().errors.map(error => error.message)).toEqual(['pull request #7 in o/r is not open for a']);
  expect({selected: store.get().selected, unsent: store.get().unsent}).toEqual({selected, unsent: null});
  expect(button(row(root), 'Chat').disabled).toBe(false);
});

test('Chat on an untracked row adds the workspace, then opens its new session on the branch', async () => {
  const stub = stubFetch(url => url.includes('action=clone') ? new Response('new-ws') : Response.json({name: 'feature', start: 'abc123'}));
  const store = tracked();
  const root = mount(store, <AllPullRequests />);
  clickControl(button(row(root, 1), 'Chat'));
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add x/y');
  clickControl(findAll(document, 'button').find(b => b.textContent === 'Clone')!);
  await flush();
  expect(Object.fromEntries(new URL(stub.mock.calls[1][0], 'http://x/').searchParams))
    .toEqual({action: 'pull-request-branch', id: 'new-ws', account: 'a', repo: 'x/y', number: '2'});
  expect(store.get().unsent).toMatchObject({workspace: 'new-ws', worktree: true, branch: {name: 'feature', start: 'abc123'}});
});

test('an untracked row stays disabled while the chat after an add is starting', async () => {
  let answer!: (response: Response) => void;
  stubFetch(url => url.includes('action=clone') ? new Response('new-ws') : new Promise<Response>(resolve => { answer = resolve; }));
  const root = mount(tracked(), <AllPullRequests />);
  clickControl(button(row(root, 1), 'Chat'));
  clickControl(findAll(document, 'button').find(b => b.textContent === 'Clone')!);
  await flush();
  expect(button(row(root, 1), 'Chat').disabled).toBe(true);
  await act(async () => answer(Response.json({name: 'feature', start: 'abc123'})));
  expect(button(row(root, 1), 'Chat').disabled).toBe(false);
});

test('closing the dialog returns focus to Chat and allows opening it again', async () => {
  const root = view([account('a', [pr({repo: 'o/r'})])]);
  const opener = button(row(root), 'Chat');
  act(() => opener.focus());
  clickControl(opener);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add o/r');
  await act(async () => {
    clickControl(findAll(document, 'button').find(b => b.textContent === 'Cancel')!);
    await new Promise(resolve => setTimeout(resolve, 0));
  });
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(document.activeElement).toBe(opener);
  clickControl(opener);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Add o/r');
});

test('collapsing a group hides its rows and is remembered', () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w'})], accounts: [account('a', [pr({repo: 'o/r'})])]}));
  const root = mount(store, <AllPullRequests />);
  clickControl(findAll(root, '[aria-expanded]')[0]);
  expect(store.get().workbench.collapsedPullRequestGroups).toEqual({review: true});
  expect(root.querySelector('[aria-label="To review"]')).toBeNull();
});

test('one pull request under two accounts is two rows', () => {
  const shared = pr({url: 'https://github.com/o/r/pull/9', repo: 'o/r'});
  expect(rows(view([account('a', [shared]), account('b', [shared])]), 'To review')).toHaveLength(2);
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
  expect(rows(root, 'To review')).toHaveLength(1);
});

test('before any account has been read it says GitHub is being checked', () => {
  expect(view([account('a', [], {updated: 0})]).textContent).toContain('Checking GitHub.');
});
