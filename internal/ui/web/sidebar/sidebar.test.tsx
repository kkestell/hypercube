import {afterEach, expect, mock, test} from 'bun:test';
import {act, useState} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testSession, testState, testWorkspace} from '../../test-state.ts';
import {nativeShortcut, type ShortcutActions} from '../app/shortcuts.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import type {GitHubAccount, GitHubStatus, PullRequest} from '../github/types.ts';
import {selectSession, selectWorkspace} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {createStore} from '../store.ts';
import {Sidebar} from './sidebar.tsx';

afterEach(() => {
  mock.restore();
  unmountAll();
  delete globalThis.hypercube;
});

const session = (id: string, extra: Partial<Session> = {}) => testSession({id, title: 'Session ' + id, ...extra});
function setup(sessions: Session[], selected: string | null = null) {
  let respond = () => new Response('');
  const requests: URL[] = [];
  stubFetch(url => {
    requests.push(new URL(url, 'http://x/'));
    return respond();
  });
  let state = testState({workspaces: [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})]});
  if (selected) state = selectSession(state, selected);
  const store = createStore(state);
  function View() {
    const [searchOpen, setSearchOpen] = useState(false);
    return <><Sidebar searchOpen={searchOpen} onSearchOpenChange={setSearchOpen} onSettings={() => {}} /><ConfirmDialog /></>;
  }
  const root = mount(store, <View />);
  const posted = () => requests.map(url => Object.fromEntries(url.searchParams));
  return {store, root, posted, answer: (next: () => Response) => { respond = next; }};
}

const click = (element: HTMLElement) => act(() => clickControl(element));
const sessionRows = (root: HTMLElement) => findAll(root, '[data-session]').map(row => row.dataset.session);
const row = (root: HTMLElement, id: string) => find(root, `[data-session="${id}"]`);
const pick = (label: string) => {
  const item = findAll(document, '[role=menuitem]').find(item => item.textContent === label);
  if (!item) throw new Error('No menu item ' + label);
  clickControl(item);
};
const key = (element: HTMLElement, key: string) => act(() => element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
function rename(root: HTMLElement, id: string) {
  key(row(root, id), 'F2');
  return element(root, 'input', HTMLInputElement);
}

const contextMenu = (element: HTMLElement) => act(() => element.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
const choose = (root: HTMLElement, selector: string, label: string) => {
  contextMenu(find(root, selector));
  pick(label);
};

const menuLabels = () => findAll(document, '[role=menuitem]').map(item => item.textContent);

const dialog = () => document.querySelector('[role=alertdialog]');
const openDialog = () => find(document, '[role=alertdialog]');
const dialogReference = (attribute: string) => document.getElementById(openDialog().getAttribute(attribute) ?? '')?.textContent;
const dialogButton = (label: string) => {
  const button = findAll(openDialog(), 'button').find(b => b.textContent === label);
  if (!button) throw new Error('No dialog button ' + label);
  return button;
};

test('workspace row background selects its dashboard without changing the action buttons', () => {
  const {store, root} = setup([session('a')], 'a');
  const header = find(root, '.workspace-section-header');
  click(find(header, '[aria-label="Collapse repo"]'));
  expect(store.get().selected).toBe('a');
  expect(store.get().workbench.collapsedWorkspaces.w).toBe(true);

  click(header);
  expect(store.get().selected).toBe('workspace:w');
  expect(store.get().workbench.collapsedWorkspaces.w).toBe(true);
  act(() => chooseSession(store, 'a'));
  click(header.firstElementChild as HTMLElement);
  expect(store.get().selected).toBe('workspace:w');

  act(() => chooseSession(store, 'a'));
  click(find(header, '[aria-label="New Session"]'));
  expect(store.get().selected).toBeNull();
  expect(store.get().unsent?.workspace).toBe('w');
  click(find(header, '[aria-label="More Actions"]'));
  expect(store.get().selected).toBeNull();
  expect(document.querySelector('[role=menu]')).not.toBeNull();
});

test('only an expanded workspace with sessions adds bottom spacing', () => {
  const {store, root} = setup([session('a')]);
  const section = find(root, '[data-workspace="w"]');
  expect(section.classList.contains('mb-2')).toBe(true);
  click(find(section, '[aria-label="Collapse repo"]'));
  expect(section.classList.contains('mb-2')).toBe(false);
  click(find(section, '[aria-label="Expand repo"]'));
  expect(section.classList.contains('mb-2')).toBe(true);
  act(() => store.update(state => ({...state, workspaces: state.workspaces.map(workspace => ({...workspace, sessions: []}))})));
  expect(section.classList.contains('mb-2')).toBe(false);
});

test('Enter saves a changed title, Escape cancels, and leaving the field saves', () => {
  const {root, posted} = setup([session('a')]);
  let field = rename(root, 'a');
  expect(document.activeElement).toBe(field);
  setValue(field, '  Renamed  ');
  key(field, 'Enter');
  expect(posted()).toEqual([{action: 'rename', id: 'a', title: 'Renamed'}]);
  expect(root.querySelector('input')).toBeNull();

  field = rename(root, 'a');
  setValue(field, 'Ignored');
  key(field, 'Escape');
  expect(posted().length).toBe(1);

  field = rename(root, 'a');
  setValue(field, 'Blurred');
  act(() => field.dispatchEvent(new FocusEvent('focusout', {bubbles: true})));
  expect(posted()[1]).toEqual({action: 'rename', id: 'a', title: 'Blurred'});

  field = rename(root, 'a');
  setValue(field, 'Session a');
  key(field, 'Enter');
  expect(posted().length).toBe(2);
});

test('an active family hides its ended children until its menu shows them, and menus offer Restart Agent only to sessions that can run', () => {
  const live = session('live', {parent: 'a'});
  const ended = session('ended', {parent: 'a', ended: true});
  const {root, posted} = setup([session('a', {activity: 'exited', children: [ended, live]}), session('b'), session('old', {ended: true})], 'live');
  expect(sessionRows(root)).toEqual(['a', 'live', 'b']);
  choose(root, '[data-session="a"]', 'Show Ended Sessions');
  expect(sessionRows(root)).toEqual(['a', 'ended', 'live', 'b']);
  expect(row(root, 'ended').querySelector('.opacity-60')).not.toBeNull();
  expect(row(root, 'a').querySelector('.opacity-60')).not.toBeNull();
  contextMenu(row(root, 'ended'));
  expect(menuLabels()).not.toContain('Restart Agent');
  key(document.activeElement as HTMLElement, 'Escape');
  contextMenu(row(root, 'a'));
  expect(menuLabels()).toEqual(['Rename', 'Restart Agent', 'Hide Ended Sessions', 'Archive Session', 'Delete Session']);
  pick('Hide Ended Sessions');
  expect(sessionRows(root)).toEqual(['a', 'live', 'b']);
  contextMenu(row(root, 'b'));
  expect(menuLabels()).toEqual(['Rename', 'Restart Agent', 'Archive Session', 'Delete Session']);
  key(document.activeElement as HTMLElement, 'Escape');
  choose(root, '[data-session="a"]', 'Archive Session');
  choose(root, '[data-session="live"]', 'Restart Agent');
  click(find(root, '[role=tab][data-state=inactive]'));
  choose(root, '[data-session="old"]', 'Unarchive');
  expect(posted()).toEqual([{action: 'archive', id: 'a'}, {action: 'restart', id: 'live'}, {action: 'unarchive', id: 'old'}]);
});


test.each([
  ['darwin', ['Open in VS Code', 'Reveal in Finder', 'Open in Terminal']],
  ['linux', ['Open in VS Code', 'Show in Files']],
])('on %s the menus ask the desktop app to open folders', async (platform, labels) => {
  const open = mock(async (_app: string, _path: string) => '');
  globalThis.hypercube = testBridge({platform, open});
  const {root, posted} = setup([session('a', {dir: '/repo/.worktrees/a'})]);
  contextMenu(row(root, 'a'));
  expect(menuLabels()).toEqual(['Rename', 'Restart Agent', ...labels, 'Archive Session', 'Delete Session']);
  await act(async () => pick('Open in VS Code'));
  contextMenu(find(root, '[data-workspace="w"]'));
  expect(menuLabels()).toEqual([...labels, 'Delete Workspace']);
  await act(async () => pick(labels[1]));
  expect(open.mock.calls).toEqual([['vscode', '/repo/.worktrees/a'], ['folder', '/repo']]);
  expect(posted()).toEqual([]);
});

test('a refused open shows the desktop app\'s message', async () => {
  globalThis.hypercube = testBridge({open: async () => 'VS Code was not found.'});
  const {root, store} = setup([session('a')]);
  contextMenu(row(root, 'a'));
  await act(async () => pick('Open in VS Code'));
  expect(store.get().errors.map(error => error.message)).toEqual(['VS Code was not found.']);
});

test.each([false, true])('session shortcuts skip hidden ended children and wrap (running child: %s)', runningChild => {
  const ended = session('ended', {parent: 'a', ended: true});
  const children = runningChild ? [session('live', {parent: 'a'}), ended] : [ended];
  const {root, store} = setup([session('a', {children}), session('b')], 'a');
  expect(sessionRows(root)).toEqual(runningChild ? ['a', 'live', 'b'] : ['a', 'b']);
  const none = () => {};
  const actions: ShortcutActions = {focusComposer: none, showHelp: none, showSettings: none, openQuickOpen: none, openSessionSearch: none};
  for (const id of runningChild ? ['live', 'b', 'a'] : ['b', 'a']) {
    act(() => nativeShortcut('next', store, actions));
    expect(store.get().selected).toBe(id);
  }
  for (const id of runningChild ? ['b', 'live', 'a'] : ['b', 'a']) {
    act(() => nativeShortcut('previous', store, actions));
    expect(store.get().selected).toBe(id);
  }
});

test('deleting a session asks first and posts only on Delete', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-session="a"]', 'Delete Session');
  expect(openDialog().getAttribute('data-state')).toBe('open');
  expect(document.activeElement === dialogButton('Cancel')).toBe(true);
  expect(dialogReference('aria-labelledby')).toBe("Are you sure you want to delete 'Session a'?");
  expect(dialogReference('aria-describedby')).toContain('Its agent stops');
  expect(posted()).toEqual([]);
  act(() => dialogButton('Cancel').click());
  expect(dialog()).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-session="a"]', 'Delete Session');
  act(() => dialogButton('Delete').click());
  expect(dialog()).toBeNull();
  expect(posted()).toEqual([{action: 'delete', id: 'a'}]);
});

test('only one row is tabbable, and the arrow keys move and open rows', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]}), session('b')]);
  const tabbable = () => findAll(root, '[role="treeitem"][tabindex="0"]').map(r => r.dataset.session);
  expect(tabbable()).toEqual(['a']);
  act(() => row(root, 'a').focus());
  key(row(root, 'a'), 'ArrowLeft');
  expect(sessionRows(root)).toEqual(['a', 'b']);
  key(row(root, 'a'), 'ArrowRight');
  expect(sessionRows(root)).toEqual(['a', 'child', 'b']);
  key(row(root, 'a'), 'ArrowRight');
  expect(document.activeElement).toBe(row(root, 'child'));
  expect(tabbable()).toEqual(['child']);
  key(row(root, 'child'), 'ArrowLeft');
  expect(document.activeElement).toBe(row(root, 'a'));
  key(row(root, 'a'), 'End');
  expect(document.activeElement).toBe(row(root, 'b'));
  key(row(root, 'b'), 'Enter');
  expect(store.get().selected).toBe('b');
});

test('a hidden ended child stays listed while shown, and the keyboard moves between rows', () => {
  const ended = session('ended', {parent: 'a', ended: true});
  const {root} = setup([session('a', {children: [session('live', {parent: 'a'}), ended]})], 'ended');
  expect(sessionRows(root)).toEqual(['a', 'live', 'ended']);
  act(() => row(root, 'ended').focus());
  key(row(root, 'ended'), 'ArrowLeft');
  expect(document.activeElement).toBe(row(root, 'a'));
  key(row(root, 'a'), 'ArrowLeft');
  expect(sessionRows(root)).toEqual(['a']);
  key(row(root, 'a'), 'ArrowRight');
  expect(sessionRows(root)).toEqual(['a', 'live', 'ended']);
});

test.each([
  {id: 'archived', view: 'Archived', other: 'Sessions'},
  {id: 'active', view: 'Sessions', other: 'Archived'},
])('search reveals an already selected $id session after switching tabs', async ({id, view, other}) => {
  const {root, store, answer} = setup([session('active'), session('archived', {ended: true})], id);
  const tabs = find(root, '[role=tablist][aria-label="Sidebar"]');
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(view);
  click(find(tabs, '[data-state=inactive]'));
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(other);
  expect(root.querySelector(`[data-session="${id}"]`)).toBeNull();
  answer(() => Response.json([{id, title: 'Session ' + id, workspace: 'repo', text: 'matching words'}]));
  await act(async () => clickControl(find(root, '[aria-label="Search sessions"]')));
  const input = element(document, '[cmdk-input]', HTMLInputElement);
  await act(async () => { setValue(input, 'matching '); input.dispatchEvent(new Event('input', {bubbles: true})); });
  await act(async () => clickControl(find(document, '[cmdk-item]')));
  expect(store.get().selected).toBe(id);
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(view);
  expect(root.querySelector(`[data-session="${id}"]`)).not.toBeNull();
});

test('archive menu hides the family while search can select its chat', () => {
  const child = session('child', {parent: 'archived', ended: true});
  const {root, store, posted} = setup([session('active'), session('archived', {ended: true, children: [child]})]);
  expect(sessionRows(root)).toEqual(['active']);
  const tablists = findAll(root, '[role=tablist]');
  expect(tablists).toHaveLength(1);
  expect(findAll(tablists[0], '[role=tab]').map(tab => tab.getAttribute('aria-label'))).toEqual(['Sessions', 'Archived']);
  choose(root, '[data-session="active"]', 'Archive Session');
  expect(posted()).toEqual([{action: 'archive', id: 'active'}]);
  act(() => chooseSession(store, 'child'));
  expect(store.get().selected).toBe('child');
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  click(find(root, '[role=tab][data-state=inactive]'));
  expect(sessionRows(root)).toEqual(['active']);
  click(find(root, '[role=tab][data-state=inactive]'));
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  expect(posted()).toHaveLength(1);
});

test('an initially selected archived child opens its family in Archived', () => {
  const child = session('child', {parent: 'archived', ended: true});
  const {root} = setup([session('active'), session('archived', {ended: true, children: [child]})], 'child');
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  expect(find(root, '[role=tablist][aria-label="Sidebar"] [data-state=active]').getAttribute('aria-label')).toBe('Archived');
});


test.each([null, 'parked'])('parked sessions dim without dimming running sessions (selected: %s)', selected => {
  const {root} = setup([
    session('busy', {activity: 'busy', children: [session('child', {parent: 'busy', activity: 'exited'})]}),
    session('idle', {activity: 'idle'}),
    session('parked', {activity: 'exited'}),
  ], selected);
  for (const id of ['parked', 'child']) {
    expect(find(row(root, id), '.session-item-status').classList.contains('opacity-60')).toBe(true);
    expect(find(row(root, id), '.session-item-title').closest('.opacity-60')).not.toBeNull();
  }
  for (const id of ['busy', 'idle']) expect(row(root, id).querySelector('.opacity-60')).toBeNull();
});

test('a saved offer shows waiting while its session is parked', () => {
  const {root, store} = setup([session('a', {activity: 'exited'})]);
  act(() => store.update(state => ({...state, requests: [{session: 'a', item: {id: 'offer', kind: 'attention'}}]})));
  expect(row(root, 'a').querySelector('[aria-label="Waiting on you"]')).not.toBeNull();
  act(() => store.update(state => ({...state, requests: []})));
  expect(row(root, 'a').querySelector('[aria-label="Waiting on you"]')).toBeNull();
});

test('the parent chevron toggles children without selecting the parent', () => {
  const {root, store} = setup([session('a', {title: 'A long session title', children: [session('child', {parent: 'a'})]})], 'child');
  const parent = row(root, 'a');
  const title = find(parent, '.session-item-title');
  const chevron = find(parent, '.session-item-twistie');
  click(chevron);
  expect(sessionRows(root)).toEqual(['a']);
  expect(store.get().selected).toBe('child');
  click(chevron);
  expect(sessionRows(root)).toEqual(['a', 'child']);
  act(() => parent.dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(sessionRows(root)).toEqual(['a']);
  click(title);
  expect(store.get().selected).toBe('a');
});

const needsYou = testPullRequest({author: 'bob', requested: 'user'});
const status = (pullRequests: PullRequest[]): GitHubStatus => ({
  repo: 'o/r', url: 'https://github.com/o/r', account: 'me', branch: 'main', ahead: 0, behind: 0, pullRequests, updated: 1,
});

function withGitHub(pullRequests: PullRequest[], accounts: GitHubAccount[] = [{login: 'me', pullRequests, updated: 1}]) {
  const store = createStore(testState({selected: '', workspaces: [testWorkspace({id: 'w', name: 'repo', github: status(pullRequests)})], accounts}));
  const root = mount(store, <Sidebar searchOpen={false} onSearchOpenChange={() => {}} onSettings={() => {}} />);
  return {store, root};
}

test.each([
  [1, '1 pull request needs you'],
  [2, '2 pull requests need you'],
])('a workspace shows how many pull requests need the account', (count, label) => {
  const {root} = withGitHub(Array.from({length: count}, (_, i) => ({...needsYou, number: i + 1, url: `https://github.com/o/r/pull/${i + 1}`})));
  const badge = find(find(root, '[data-workspace="w"]'), `[aria-label="${label}"]`);
  expect(badge.textContent).toBe(String(count));
  expect(badge.getAttribute('title')).toBe(label);
});

test('a workspace with nothing that needs the account shows no count', () => {
  const {root} = withGitHub([testPullRequest({author: 'me', merge: 'BLOCKED'})]);
  expect(find(root, '[data-workspace="w"]').textContent).not.toMatch(/need/);
  expect(findAll(root, '[data-workspace="w"] [role=img]')).toHaveLength(0);
});

test('the Pull Requests button shows the home view and counts what needs the user', () => {
  const {store, root} = withGitHub([needsYou]);
  const button = findAll(root, 'button').find(b => b.textContent?.startsWith('Pull Requests'))!;
  expect(button.textContent).toBe('Pull Requests1');
  expect(button.getAttribute('aria-pressed')).toBe('false');
  act(() => store.update(state => selectWorkspace(state, 'w')));
  click(button);
  expect(store.get().selected).toBeNull();
  expect(button.getAttribute('aria-pressed')).toBe('true');
});

test('the Pull Requests button is absent without workspaces or accounts', () => {
  for (const state of [testState({workspaces: [], accounts: [{login: 'me', pullRequests: [], updated: 1}]}),
    testState({workspaces: [testWorkspace({id: 'w'})], accounts: []})]) {
    const root = mount(createStore(state), <Sidebar searchOpen={false} onSearchOpenChange={() => {}} onSettings={() => {}} />);
    expect(findAll(root, 'button').some(b => b.textContent?.startsWith('Pull Requests'))).toBe(false);
    unmountAll();
  }
});
