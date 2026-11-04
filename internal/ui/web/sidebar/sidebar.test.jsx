import {setValue, clickControl} from '../../test-render.jsx';
import {afterEach, expect, spyOn, test} from 'bun:test';
import {act, useState} from 'react';
import {mount, unmountAll} from '../../test-render.jsx';
import {ConfirmDialog} from '../shared/confirm-dialog.jsx';
import {createStore} from '../store.js';
import {Sidebar} from './sidebar.jsx';

const listeners = [];
const observeKey = listener => { addEventListener('keydown', listener); listeners.push(listener); };
let spies = [];
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  for (const listener of listeners.splice(0)) removeEventListener('keydown', listener);
  unmountAll();
});

const session = (id, extra = {}) => ({id, title: 'Session ' + id, running: true, unseen: false, children: [], activity: 'idle', ...extra});
const state = sessions => ({workspaces: [{id: 'w', name: 'repo', path: '/repo', sessions}], limits: [], selected: null, unsent: null, session: null, confirmation: null, overlays: {}});

function setup(sessions) {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const store = createStore(state(sessions));
  function View() {
    const [searchOpen, setSearchOpen] = useState(false);
    return <><Sidebar searchOpen={searchOpen} onSearchOpenChange={setSearchOpen} /><ConfirmDialog /></>;
  }
  const root = mount(store, <View />);
  const posted = () => fetches.mock.calls.map(([url]) => Object.fromEntries(new URL(url, 'http://x/').searchParams));
  return {store, root, posted, fetches};
}

const click = element => act(() => clickControl(element));
const sessionRows = root => [...root.querySelectorAll('[data-session]')].map(row => row.dataset.session);
const row = (root, id) => root.querySelector(`[data-session="${id}"]`);
const select = (root, id) => click(row(root, id));
const dots = (root, id) => [...root.querySelectorAll(`[data-session="${id}"] .session-item-status [role=img]`)]
  .map(icon => ({'Waiting on you': 'waiting', 'New reply': 'unseen', Exited: 'exited'})[icon.getAttribute('aria-label')]).filter(Boolean);
const pick = (root, index) => clickControl(document.querySelectorAll('[role=menuitem]')[index]);
const workspaceMenu = (root, id = 'w') => click(root.querySelector(`[data-workspace="${id}"] [title="More Actions"]`));
test('running sessions come before stopped sessions, preserving order within each group', () => {
  const sessions = Array.from({length: 23}, (_, i) => session('s' + (22 - i), {running: i % 2 === 0}));
  const {store, root} = setup(sessions);
  const running = sessions.filter(s => s.running).map(s => s.id);
  const stopped = sessions.filter(s => !s.running).map(s => s.id);
  expect(sessionRows(root)).toEqual([...running, ...stopped]);
  expect(row(root, 's21').classList.contains('opacity-60')).toBe(true);
  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: sessions.map(item => ({...item, running: !item.running}))}]}));
  expect(sessionRows(root)).toEqual([...stopped, ...running]);
  expect(row(root, 's22').classList.contains('opacity-60')).toBe(true);
});

test('a running child keeps its stopped parent ahead of stopped sessions', () => {
  const {root} = setup([
    session('stopped', {running: false}),
    session('parent', {running: false, children: [session('child', {parent: 'parent'})]}),
    session('running'),
  ]);
  expect(sessionRows(root)).toEqual(['parent', 'child', 'running', 'stopped']);
});

test('Add Workspace works even when no workspaces exist', () => {
  const {root, store, posted} = setup([]);
  store.update(s => ({...s, workspaces: []}));
  click(root.querySelector('[title="Add Workspace"]'));
  expect(posted()).toEqual([{action: 'new'}]);
});

test('clicking a session row chooses it, and a stopped one starts', () => {
  const {store, root, posted} = setup([session('a'), session('b', {running: false})]);
  select(root, 'b');
  expect(store.get().selected).toBe('b');
  expect(posted()).toEqual([{action: 'start', id: 'b'}]);
});

const key = (element, key) => act(() => element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
function rename(root, id) {
  key(row(root, id), 'F2');
  return root.querySelector('input');
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
  const reached = [];
  const field = rename(root, 'a');
  observeKey(e => reached.push(e.key));
  key(field, ' ');
  key(field, 'ArrowDown');
  expect(reached).toEqual([]);
});

const choose = (root, selector, index) => {
  act(() => root.querySelector(selector).dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  pick(root, index);
};

test('a session\'s menu stops a running session and starts a stopped or failed one', () => {
  const {root, posted} = setup([
    session('running'),
    session('stopped', {running: false}),
    session('failed', {activity: 'exited'}),
  ]);
  choose(root, '[data-session="running"]', 0);
  choose(root, '[data-session="stopped"]', 0);
  choose(root, '[data-session="failed"]', 0);
  expect(posted()).toEqual([
    {action: 'stop', id: 'running'},
    {action: 'start', id: 'stopped'},
    {action: 'start', id: 'failed'},
  ]);
  expect(document.querySelector('[role=menu]')).toBeNull();
});

const dialog = root => document.querySelector('[role=alertdialog]');
const press = (root, key) => act(() => dialog(root).querySelector('button').dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
const dialogButton = (root, label) => [...dialog(root).querySelectorAll('button')].find(b => b.textContent === label);

test('deleting a session asks first and posts only on Delete', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-session="a"]', 3);
  expect(dialog(root).getAttribute('data-state')).toBe('open');
  expect(document.activeElement === dialogButton(root, 'Cancel')).toBe(true);
  expect(document.getElementById(dialog(root).getAttribute('aria-labelledby')).textContent).toBe("Are you sure you want to delete 'Session a'?");
  expect(document.getElementById(dialog(root).getAttribute('aria-describedby')).textContent).toContain('Its agent stops');
  expect(posted()).toEqual([]);
  act(() => dialogButton(root, 'Cancel').click());
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-session="a"]', 3);
  act(() => dialogButton(root, 'Delete').click());
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([{action: 'delete', id: 'a'}]);
});

test('Escape cancels deleting a workspace', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-workspace="w"]', 0);
  expect(dialog(root).querySelector('[data-slot=alert-dialog-title]').textContent).toBe('Are you sure you want to delete \'repo\'?');
  press(root, 'Escape');
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-workspace="w"]', 0);
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'w'}]);
});

test('nested sessions stay in creation order and ended ones dim', () => {
  const children = [session('second', {parent: 'a'}), session('first', {parent: 'a', running: false, ended: true})];
  const {root} = setup([session('a', {children}), session('p0')]);
  expect(sessionRows(root)).toEqual(['a', 'second', 'first', 'p0']);
  expect(root.querySelector('[data-session="first"]').classList.contains('opacity-60')).toBe(true);
});

test('clicking a parent selects it and keeps its children visible', () => {
  const {store, root, posted} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  select(root, 'a');
  expect(store.get().selected).toBe('a');
  expect(sessionRows(root)).toEqual(['a', 'child']);
  expect(posted()).toEqual([]);
});

test('all children show under a stopped parent and stopped and ended children dim', () => {
  const children = [session('running', {parent: 'a'}), session('stopped', {parent: 'a', running: false}), session('ended', {parent: 'a', ended: true})];
  const {root} = setup([session('a', {running: false, children})]);
  expect(sessionRows(root)).toEqual(['a', 'running', 'stopped', 'ended']);
  for (const id of ['a', 'stopped', 'ended']) expect(row(root, id).classList.contains('opacity-60')).toBe(true);
  expect(row(root, 'running').classList.contains('opacity-60')).toBe(false);
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
  expect(dots(root, 'exited')).toEqual(['exited']);
  expect(row(root, 'exited').querySelector('.session-item-decoration').textContent).toBe('');
  expect(root.querySelector('[data-variant=destructive]')).toBeNull();
  store.update(s => ({...s, selected: 'replied'}));
  expect(dots(root, 'replied')).toEqual([]);
});

test.each([
  ['idle', 'idle', []],
  ['idle', 'busy', ['child']],
  ['busy', 'idle', ['a']],
  ['busy', 'busy', ['a', 'child']],
])('each row spins only for its own work: parent=%s, child=%s', (parent, child, expected) => {
  const {root} = setup([session('a', {activity: parent, children: [session('child', {parent: 'a', activity: child})]})]);
  const busy = [...root.querySelectorAll('.session-item-busy')].map(icon => icon.closest('[data-session]').dataset.session);
  expect(busy).toEqual(expected);
});

test('waiting and unseen indicators take priority over the working indicator', () => {
  const {root} = setup([session('a', {activity: 'busy', unseen: true, children: [
    session('child', {parent: 'a', activity: 'busy', attention: true}),
  ]})]);
  expect(dots(root, 'a')).toEqual(['unseen']);
  expect(dots(root, 'child')).toEqual(['waiting']);
});

test('session actions and workspace actions open their respective menus', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'})]})]);
  choose(root, '[data-session="a"]', 0);
  expect(posted()).toEqual([{action: 'stop', id: 'a'}]);
  workspaceMenu(root);
  pick(root, 0);
  expect(dialog(root).querySelector('[data-slot=alert-dialog-title]').textContent).toBe('Are you sure you want to delete \'repo\'?');
});

test('Escape closes a menu and returns the focus to its button', async () => {
  const {root} = setup([session('a')]);
  const button = root.querySelector('[data-workspace="w"] [title="More Actions"]');
  button.tabIndex = 0;
  button.focus();
  click(button);
  const menu = document.querySelector('[role=menu]');
  await act(async () => {});
  expect(menu.contains(document.activeElement)).toBe(true);
  act(() => menu.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(document.querySelector('[role=menu]')).toBeNull();
  await act(() => new Promise(resolve => setTimeout(resolve)));
  expect(document.activeElement === button).toBe(true);
});

test('active nested sessions offer only Rename and ended ones also Delete Session', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'}), session('ended', {parent: 'a', ended: true, running: false})]})]);
  act(() => root.querySelector('[data-session="active"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  expect([...document.querySelectorAll('[role=menuitem]')].map(item => item.textContent)).toEqual(['Rename', 'Open in VS Code', 'Reveal in Finder', 'Open in Terminal']);
  choose(root, '[data-session="ended"]', 1);
  expect(dialog(root).querySelector('[data-slot=alert-dialog-description]').textContent).toBe('Its chat is deleted.');
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'ended'}]);
});

test('a worktree session reports its branch and merge state on an icon without visible status text', () => {
  const merge = extra => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: true, uncommitted: 0, ...extra});
  const {store, root} = setup([session('a', {worktree: true, merge: merge({unmerged: 1})}), session('b')]);
  const badge = () => root.querySelector('[data-session="a"] .session-item-branch');
  expect(badge().title).toContain('hypercube/session-a');
  expect(badge().textContent).toBe('');
  expect(badge().title).toContain('1 commit not merged into main');
  expect(root.querySelector('[data-session="b"] .session-item-branch')).toBeNull();
  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {worktree: true, merge: merge()})]}]}));
  expect(badge().textContent).toBe('');
  expect(badge().title).toContain('Merged into main');
});

test('each workspace has a section, with children inside their parent\'s row', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]}), session('b')]);
  store.update(s => ({...s, workspaces: [...s.workspaces, {id: 'other', name: 'other', path: '/other', sessions: [session('c')]}]}));
  expect(sessionRows(root)).toEqual(['a', 'child', 'b', 'c']);
  expect([...root.querySelectorAll('.workspace-section-header')].map(header => [header.textContent, header.title])).toEqual([
    ['repo', '/repo'], ['other', '/other'],
  ]);
  expect(['a', 'child', 'b'].map(id => row(root, id).getAttribute('aria-level'))).toEqual(['1', '2', '1']);
  expect(row(root, 'a').getAttribute('aria-expanded')).toBe('true');
  expect(row(root, 'b').hasAttribute('aria-expanded')).toBe(false);
});

test('a parent\'s chevron and double-click collapse and expand its children, and a click only selects', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  const chevron = () => row(root, 'a').querySelector('.session-item-twistie');
  click(chevron());
  expect(sessionRows(root)).toEqual(['a']);
  expect(row(root, 'a').getAttribute('aria-expanded')).toBe('false');
  expect(store.get().selected).toBeNull();
  act(() => row(root, 'a').dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(sessionRows(root)).toEqual(['a', 'child']);
  select(root, 'a');
  expect(sessionRows(root)).toEqual(['a', 'child']);
  expect(store.get().selected).toBe('a');
});

test('a section header is a labelled toggle that Enter, Space, Left, and Right drive', () => {
  const {root} = setup([session('a')]);
  const toggle = root.querySelector('.workspace-section-toggle');
  expect(toggle.tagName).toBe('BUTTON');
  expect(root.querySelector('[data-workspace="w"]').getAttribute('aria-labelledby')).toBe(toggle.id);
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
  store.update(s => ({...s, workspaces: [...s.workspaces, {id: 'other', name: 'other', path: '/other', sessions: []}]}));
  click(root.querySelector('[data-workspace="other"] [title="New Session"]'));
  expect(store.get().unsent).toMatchObject({workspace: 'other'});
});

test('only one row is tabbable, and the arrow keys move and open rows', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]}), session('b')]);
  const tabbable = () => [...root.querySelectorAll('[role="treeitem"][tabindex="0"]')].map(r => r.dataset.session);
  expect(tabbable()).toEqual(['a']);
  row(root, 'a').focus();
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
  row(root, 'b').focus();
  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a')]}]}));
  expect(sessionRows(root)).toEqual(['a']);
  expect(document.activeElement).toBe(row(root, 'a'));
  expect(row(root, 'a').tabIndex).toBe(0);
});

test('Escape on a row reaches the app, and the rename field keeps its own', () => {
  const {root} = setup([session('a')]);
  const reached = [];
  observeKey(e => reached.push([e.key, e.defaultPrevented]));
  row(root, 'a').focus();
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
])('%o on a focused row opens its menu', init => {
  const {root} = setup([session('a')]);
  act(() => row(root, 'a').dispatchEvent(new KeyboardEvent('keydown', {...init, bubbles: true, cancelable: true})));
  expect([...document.querySelectorAll('[role=menuitem]')].map(item => item.textContent)).toEqual(['Stop Session', 'Rename', 'Archive Session', 'Delete Session', 'Open in VS Code', 'Reveal in Finder', 'Open in Terminal']);
});

test('workspace actions remain available while their menu is open', () => {
  const {root} = setup([session('a')]);
  workspaceMenu(root);
  expect(root.querySelector('[title="More Actions"]').hidden).toBe(false);
  expect(document.querySelector('[role=menu]')).not.toBeNull();
});

test('rows carry their place among their siblings', () => {
  const {root} = setup([session('a', {children: [session('c1', {parent: 'a'}), session('c2', {parent: 'a'})]}), session('b')]);
  const place = id => [row(root, id).getAttribute('aria-posinset'), row(root, id).getAttribute('aria-setsize')];
  expect(['a', 'c1', 'c2', 'b'].map(place)).toEqual([['1', '2'], ['1', '2'], ['2', '2'], ['2', '2']]);
});

test('Rename in a session\'s menu opens the rename field', () => {
  const {root} = setup([session('a')]);
  choose(root, '[data-session="a"]', 1);
  expect(row(root, 'a').querySelector('input').getAttribute('value')).toBe('Session a');
});

test('workspace headers offer the workspace context menu', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-workspace="w"]', 0);
  expect(dialog(root).querySelector('[data-slot=alert-dialog-title]').textContent).toBe("Are you sure you want to delete 'repo'?");
  click(dialogButton(root, 'Delete'));
  expect(posted()).toEqual([{action: 'delete', id: 'w'}]);
});

test('search opens from the sidebar and choosing a result shows its session', async () => {
  const {root, store, fetches} = setup([session('a'), session('ended', {ended: true, running: false})]);
  fetches.mockImplementation(async () => Response.json([{id: 'ended', title: 'Session ended', workspace: 'repo', text: 'matching words'}]));
  await act(async () => clickControl(root.querySelector('[aria-label="Search sessions"]')));
  const input = document.querySelector('[cmdk-input]');
  expect(input).not.toBeNull();
  await act(async () => { setValue(input, 'matching '); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(document.body.textContent).toContain('matching words');
  await act(async () => clickControl(document.querySelector('[cmdk-item]')));
  expect(store.get().selected).toBe('ended');
});

test('closing search returns focus to its button', async () => {
  const {root, store} = setup([session('a')]);
  const button = root.querySelector('[aria-label="Search sessions"]');
  act(() => button.focus());
  await act(async () => clickControl(button));
  const input = document.querySelector('[cmdk-input]');
  expect(input).not.toBeNull();
  expect(Object.keys(store.get().overlays)).toHaveLength(1);
  await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(document.activeElement).toBe(button);
  expect(Object.keys(store.get().overlays)).toHaveLength(0);
});


test('archive menu hides the family while search can select its chat', async () => {
  const child = session('child', {parent: 'archived', running: false, ended: true});
  const {root, store, posted} = setup([session('active'), session('archived', {running: false, ended: true, children: [child]})]);
  expect(sessionRows(root)).toEqual(['active']);
  choose(root, '[data-session="active"]', 2);
  expect(posted()).toEqual([{action: 'archive', id: 'active'}]);
  const {chooseSession} = await import('../sessions/session-actions.js');
  chooseSession(store, 'child');
  expect(store.get().selected).toBe('child');
  expect(posted()).toHaveLength(1);
});
