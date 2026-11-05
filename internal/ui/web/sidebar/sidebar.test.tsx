import {afterEach, expect, mock, test} from 'bun:test';
import {act, useState} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import type {Activity, Merge, Session} from '../sessions/types.ts';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {createStore, type Store} from '../store.ts';
import {Sidebar} from './sidebar.tsx';

const listeners: ((event: KeyboardEvent) => void)[] = [];
const observeKey = (listener: (event: KeyboardEvent) => void) => { addEventListener('keydown', listener); listeners.push(listener); };
afterEach(() => {
  mock.restore();
  for (const listener of listeners.splice(0)) removeEventListener('keydown', listener);
  unmountAll();
});

const session = (id: string, extra: Partial<Session> = {}) => testSession({id, title: 'Session ' + id, running: true, ...extra});
const setSessions = (store: Store, sessions: Session[]) =>
  act(() => store.update(s => ({...s, workspaces: s.workspaces.map(w => w.id === 'w' ? {...w, sessions} : w)})));

function setup(sessions: Session[]) {
  let respond = () => new Response('');
  const requests: URL[] = [];
  stubFetch(url => {
    requests.push(new URL(url, 'http://x/'));
    return respond();
  });
  const store = createStore(testState({selected: null, workspaces: [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})]}));
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
const select = (root: HTMLElement, id: string) => click(row(root, id));
const statusLabels: Record<string, string> = {'Waiting on you': 'waiting', 'New reply': 'unseen'};
const dots = (root: HTMLElement, id: string) => findAll(root, `[data-session="${id}"] .session-item-status [role=img]`)
  .map(icon => statusLabels[icon.getAttribute('aria-label') ?? '']).filter(Boolean);
const pick = (label: string) => {
  const item = findAll(document, '[role=menuitem]').find(item => item.textContent === label);
  if (!item) throw new Error('No menu item ' + label);
  clickControl(item);
};
const workspaceMenu = (root: HTMLElement, id = 'w') => click(find(root, `[data-workspace="${id}"] [title="More Actions"]`));
const showStopped = (root: HTMLElement, selector = '[data-workspace="w"] .workspace-section-header') =>
  choose(root, selector, 'Show Stopped Sessions');
test('running sessions come before stopped sessions, preserving order within each group', () => {
  const sessions = Array.from({length: 23}, (_, i) => session('s' + (22 - i), {running: i % 2 === 0}));
  const {store, root} = setup(sessions);
  showStopped(root);
  const running = sessions.filter(s => s.running).map(s => s.id);
  const stopped = sessions.filter(s => !s.running).map(s => s.id);
  expect(sessionRows(root)).toEqual([...running, ...stopped]);
  setSessions(store, sessions.map(item => ({...item, running: !item.running})));
  expect(sessionRows(root)).toEqual([...stopped, ...running]);
});

test('a running child keeps its stopped parent visible, with stopped children shown in creation order on request', () => {
  const {root} = setup([
    session('stopped', {running: false}),
    session('parent', {running: false, children: [
      session('child', {parent: 'parent'}),
      session('second', {parent: 'parent', running: false}),
      session('first', {parent: 'parent', running: false, ended: true}),
    ]}),
    session('running'),
  ]);
  expect(sessionRows(root)).toEqual(['parent', 'child', 'running']);
  showStopped(root, '[data-session="parent"]');
  expect(sessionRows(root)).toEqual(['parent', 'child', 'second', 'first', 'running']);
  showStopped(root);
  expect(sessionRows(root)).toEqual(['parent', 'child', 'second', 'first', 'running', 'stopped']);
});

test('stopped sessions hide until a menu shows them, and the menu hides them again', () => {
  const {root} = setup([session('a', {children: [session('done', {parent: 'a', running: false})]}), session('stopped', {running: false})]);
  expect(sessionRows(root)).toEqual(['a']);
  showStopped(root, '[data-session="a"]');
  expect(sessionRows(root)).toEqual(['a', 'done']);
  showStopped(root);
  expect(sessionRows(root)).toEqual(['a', 'done', 'stopped']);
  choose(root, '[data-workspace="w"] .workspace-section-header', 'Hide Stopped Sessions');
  choose(root, '[data-session="a"]', 'Hide Stopped Sessions');
  expect(sessionRows(root)).toEqual(['a']);
});

test('stopped sessions show no attention dots', () => {
  const {root} = setup([session('a', {running: false, attention: true}), session('b', {running: false, unseen: true})]);
  showStopped(root);
  expect(dots(root, 'a')).toEqual([]);
  expect(dots(root, 'b')).toEqual([]);
});

test('Add Workspace works even when no workspaces exist', () => {
  const {root, store, posted} = setup([]);
  act(() => store.update(s => ({...s, workspaces: []})));
  click(find(root, '[title="Add Workspace"]'));
  expect(posted()).toEqual([{action: 'new'}]);
});

test('clicking a session row chooses it, and a stopped one starts', () => {
  const {store, root, posted} = setup([session('a'), session('b', {running: false})]);
  showStopped(root);
  select(root, 'b');
  expect(store.get().selected).toBe('b');
  expect(posted()).toEqual([{action: 'start', id: 'b'}]);
});

const key = (element: HTMLElement, key: string) => act(() => element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
function rename(root: HTMLElement, id: string) {
  key(row(root, id), 'F2');
  return element(root, 'input', HTMLInputElement);
}

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

test('keys typed in the rename field do not reach the sidebar', () => {
  const {root} = setup([session('a')]);
  const reached: string[] = [];
  const field = rename(root, 'a');
  observeKey(e => reached.push(e.key));
  key(field, ' ');
  key(field, 'ArrowDown');
  expect(reached).toEqual([]);
});

const contextMenu = (element: HTMLElement) => act(() => element.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
const choose = (root: HTMLElement, selector: string, label: string) => {
  contextMenu(find(root, selector));
  pick(label);
};

test('a session\'s menu stops a running session and starts a stopped or failed one', () => {
  const {root, posted} = setup([
    session('running'),
    session('stopped', {running: false}),
    session('failed', {activity: 'exited'}),
  ]);
  showStopped(root);
  choose(root, '[data-session="running"]', 'Stop Session');
  choose(root, '[data-session="stopped"]', 'Start Session');
  choose(root, '[data-session="failed"]', 'Start Session');
  expect(posted().slice(-3)).toEqual([
    {action: 'stop', id: 'running'},
    {action: 'start', id: 'stopped'},
    {action: 'start', id: 'failed'},
  ]);
  expect(document.querySelector('[role=menu]')).toBeNull();
});

const dialog = () => document.querySelector('[role=alertdialog]');
const openDialog = () => find(document, '[role=alertdialog]');
const dialogText = (selector: string) => find(openDialog(), selector).textContent;
const dialogReference = (attribute: string) => document.getElementById(openDialog().getAttribute(attribute) ?? '')?.textContent;
const press = (key: string) => act(() => find(openDialog(), 'button').dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
const dialogButton = (label: string) => {
  const button = findAll(openDialog(), 'button').find(b => b.textContent === label);
  if (!button) throw new Error('No dialog button ' + label);
  return button;
};

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

test('workspace header and action menus ask before deletion, which Escape cancels and Delete confirms', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-workspace="w"] .workspace-section-header', 'Delete Workspace');
  expect(dialogText('[data-slot=alert-dialog-title]')).toBe('Are you sure you want to delete \'repo\'?');
  press('Escape');
  expect(dialog()).toBeNull();
  expect(posted()).toEqual([]);

  workspaceMenu(root);
  pick('Delete Workspace');
  act(() => dialogButton('Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'w'}]);
});

test('each row shows only its own state, and nothing while idle', () => {
  const {store, root} = setup([session('a', {children: [
    session('asks', {parent: 'a', attention: true, activity: 'busy'}),
    session('replied', {parent: 'a', unseen: true}),
    session('exited', {parent: 'a', activity: 'exited'}),
  ]})]);
  expect(dots(root, 'a')).toEqual([]);
  expect(dots(root, 'asks')).toEqual(['waiting']);
  expect(dots(root, 'replied')).toEqual(['unseen']);
  expect(dots(root, 'exited')).toEqual([]);
  expect(row(root, 'exited').querySelector('.session-item-decoration')).toBeNull();
  expect(root.querySelector('[data-variant=destructive]')).toBeNull();
  act(() => store.update(s => ({...s, selected: 'replied'})));
  expect(dots(root, 'replied')).toEqual([]);
});

test.each<[Activity, Activity, string[]]>([
  ['idle', 'idle', []],
  ['idle', 'busy', ['child']],
  ['busy', 'idle', ['a']],
  ['busy', 'busy', ['a', 'child']],
])('each row breathes only for its own work: parent=%s, child=%s', (parent, child, expected) => {
  const {root} = setup([session('a', {activity: parent, children: [session('child', {parent: 'a', activity: child})]})]);
  const busy = findAll(root, '.busy-dot').map(icon => icon.closest<HTMLElement>('[data-session]')?.dataset.session);
  expect(busy).toEqual(expected);
});

test('waiting and unseen indicators take priority over the working indicator', () => {
  const {root} = setup([session('a', {activity: 'busy', unseen: true, children: [
    session('child', {parent: 'a', activity: 'busy', attention: true}),
  ]})]);
  expect(dots(root, 'a')).toEqual(['unseen']);
  expect(dots(root, 'child')).toEqual(['waiting']);
});

test('Escape closes a menu and returns the focus to its button', async () => {
  const {root} = setup([session('a')]);
  const button = find(root, '[data-workspace="w"] [title="More Actions"]');
  button.tabIndex = 0;
  button.focus();
  click(button);
  const menu = find(document, '[role=menu]');
  await act(async () => {});
  expect(menu.contains(document.activeElement)).toBe(true);
  act(() => menu.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(document.querySelector('[role=menu]')).toBeNull();
  await act(() => new Promise<void>(resolve => setTimeout(resolve)));
  expect(document.activeElement === button).toBe(true);
});

test('active nested sessions offer only Rename and ended ones also Delete Session', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'}), session('ended', {parent: 'a', ended: true, running: false})]})]);
  showStopped(root, '[data-session="a"]');
  contextMenu(find(root, '[data-session="active"]'));
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toEqual(['Rename', 'Open in VS Code', 'Reveal in Finder', 'Open in Terminal']);
  choose(root, '[data-session="ended"]', 'Delete Session');
  expect(dialogText('[data-slot=alert-dialog-description]')).toBe('Its chat is deleted.');
  act(() => dialogButton('Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'ended'}]);
});

test('a worktree session reports its branch and merge state on an icon without visible status text', () => {
  const merge = (extra: Partial<Merge> = {}): Merge => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: true, uncommitted: 0, ...extra});
  const {store, root} = setup([session('a', {worktree: true, merge: merge({unmerged: 1})}), session('b')]);
  const badge = () => find(root, '[data-session="a"] .session-item-branch');
  expect(badge().title).toContain('hypercube/session-a');
  expect(badge().textContent).toBe('');
  expect(badge().title).toContain('1 commit not merged into main');
  expect(root.querySelector('[data-session="b"] .session-item-branch')).toBeNull();
  setSessions(store, [session('a', {worktree: true, merge: merge()})]);
  expect(badge().textContent).toBe('');
  expect(badge().title).toContain('Merged into main');
});

test('each workspace has a section, with children inside their parent\'s row', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]}), session('b')]);
  act(() => store.update(s => ({...s, workspaces: [...s.workspaces, testWorkspace({id: 'other', name: 'other', path: '/other', sessions: [session('c')]})]})));
  expect(sessionRows(root)).toEqual(['a', 'child', 'b', 'c']);
  expect(findAll(root, '.workspace-section-header').map(header => [header.textContent, header.title])).toEqual([
    ['repo', '/repo'], ['other', '/other'],
  ]);
  expect(['a', 'child', 'b'].map(id => row(root, id).getAttribute('aria-level'))).toEqual(['1', '2', '1']);
  expect(row(root, 'a').getAttribute('aria-expanded')).toBe('true');
  expect(row(root, 'b').hasAttribute('aria-expanded')).toBe(false);
});

test('a parent\'s chevron and double-click collapse and expand its children, and a click only selects', () => {
  const {root, store, posted} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  click(find(row(root, 'a'), '.session-item-twistie'));
  expect(sessionRows(root)).toEqual(['a']);
  expect(row(root, 'a').getAttribute('aria-expanded')).toBe('false');
  expect(store.get().selected).toBeNull();
  act(() => row(root, 'a').dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(sessionRows(root)).toEqual(['a', 'child']);
  select(root, 'a');
  expect(sessionRows(root)).toEqual(['a', 'child']);
  expect(store.get().selected).toBe('a');
  expect(posted()).toEqual([]);
});

test('a section header is a labelled toggle that Enter, Space, Left, and Right drive', () => {
  const {root} = setup([session('a')]);
  const toggle = find(root, '.workspace-section-toggle');
  expect(toggle.tagName).toBe('BUTTON');
  expect(find(root, '[data-workspace="w"]').getAttribute('aria-labelledby')).toBe(toggle.id);
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  click(toggle);
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
  expect(sessionRows(root)).toEqual([]);
  expect(root.querySelector('[title="More Actions"]')).not.toBeNull();
  key(toggle, 'ArrowRight');
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  expect(sessionRows(root)).toEqual(['a']);
  key(toggle, 'ArrowLeft');
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
});

test('a section\'s New Session opens a new session in that workspace', () => {
  const {root, store} = setup([session('a')]);
  act(() => store.update(s => ({...s, workspaces: [...s.workspaces, testWorkspace({id: 'other', name: 'other', path: '/other'})]})));
  click(find(root, '[data-workspace="other"] [title="New Session"]'));
  expect(store.get().unsent).toMatchObject({workspace: 'other'});
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

test('when the focused row goes away, the focus moves to a remaining row', () => {
  const {root, store} = setup([session('a'), session('b')]);
  act(() => row(root, 'b').focus());
  setSessions(store, [session('a')]);
  expect(sessionRows(root)).toEqual(['a']);
  expect(document.activeElement).toBe(row(root, 'a'));
  expect(row(root, 'a').tabIndex).toBe(0);
});

test('Escape on a row reaches the app, and the rename field keeps its own', () => {
  const {root} = setup([session('a')]);
  const reached: [string, boolean][] = [];
  observeKey(e => reached.push([e.key, e.defaultPrevented]));
  act(() => row(root, 'a').focus());
  key(row(root, 'a'), 'Escape');
  expect(reached).toEqual([['Escape', false]]);
  const field = rename(root, 'a');
  reached.length = 0;
  key(field, 'Escape');
  expect(reached).toEqual([]);
});

test.each([
  [{key: 'F10', shiftKey: true}],
  [{key: 'ContextMenu'}],
])('%o on a focused row opens its menu', async init => {
  const {root} = setup([session('a')]);
  await act(async () => { row(root, 'a').dispatchEvent(new KeyboardEvent('keydown', {...init, bubbles: true, cancelable: true})); });
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toEqual(['Stop Session', 'Rename', 'Open in VS Code', 'Reveal in Finder', 'Open in Terminal', 'Archive Session', 'Delete Session']);
});

test('workspace actions remain available while their menu is open', async () => {
  const {root} = setup([session('a')]);
  await act(async () => { workspaceMenu(root); });
  expect(find(root, '[title="More Actions"]').hidden).toBe(false);
  expect(document.querySelector('[role=menu]')).not.toBeNull();
});

test('rows carry their place among their siblings', () => {
  const {root} = setup([session('a', {children: [session('c1', {parent: 'a'}), session('c2', {parent: 'a'})]}), session('b')]);
  const place = (id: string) => [row(root, id).getAttribute('aria-posinset'), row(root, id).getAttribute('aria-setsize')];
  expect(['a', 'c1', 'c2', 'b'].map(place)).toEqual([['1', '2'], ['1', '2'], ['2', '2'], ['2', '2']]);
});

test('Rename in a session\'s menu opens the rename field', () => {
  const {root} = setup([session('a')]);
  choose(root, '[data-session="a"]', 'Rename');
  expect(element(row(root, 'a'), 'input', HTMLInputElement).getAttribute('value')).toBe('Session a');
});

test('search opens from the sidebar and choosing a result shows its session', async () => {
  const {root, store, answer} = setup([session('a'), session('ended', {ended: true, running: false})]);
  answer(() => Response.json([{id: 'ended', title: 'Session ended', workspace: 'repo', text: 'matching words'}]));
  await act(async () => clickControl(find(root, '[aria-label="Search sessions"]')));
  const input = element(document, '[cmdk-input]', HTMLInputElement);
  await act(async () => { setValue(input, 'matching '); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(document.body.textContent).toContain('matching words');
  await act(async () => clickControl(find(document, '[cmdk-item]')));
  expect(store.get().selected).toBe('ended');
});

test('closing search returns focus to its button', async () => {
  const {root, store} = setup([session('a')]);
  const button = find(root, '[aria-label="Search sessions"]');
  act(() => button.focus());
  await act(async () => clickControl(button));
  const input = element(document, '[cmdk-input]', HTMLInputElement);
  await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(document.activeElement).toBe(button);
});

test('archive menu hides the family while search can select its chat', () => {
  const child = session('child', {parent: 'archived', running: false, ended: true});
  const {root, store, posted} = setup([session('active'), session('archived', {running: false, ended: true, children: [child]})]);
  expect(sessionRows(root)).toEqual(['active']);
  choose(root, '[data-session="active"]', 'Archive Session');
  expect(posted()).toEqual([{action: 'archive', id: 'active'}]);
  act(() => chooseSession(store, 'child'));
  expect(store.get().selected).toBe('child');
  expect(posted()).toHaveLength(1);
});
