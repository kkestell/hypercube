import '@vscode-elements/elements/dist/vscode-context-menu/index.js';
import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {ConfirmDialog} from '../shared/confirm-dialog.jsx';
import {createStore} from '../store.js';
import {Sidebar} from './sidebar.jsx';

let spies = [];
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  unmountAll();
});

const session = (id, extra = {}) => ({id, title: 'Session ' + id, running: true, unseen: false, children: [], activity: 'idle', ...extra});
const state = sessions => ({workspaces: [{id: 'w', name: 'repo', path: '/repo', sessions}], selected: null, unsent: null, session: null, confirmation: null, overlays: {}});

function setup(sessions) {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const store = createStore(state(sessions));
  const root = mount(store, <><Sidebar /><ConfirmDialog /></>);
  const posted = () => fetches.mock.calls.map(([url]) => Object.fromEntries(new URL(url, 'http://x/').searchParams));
  return {store, root, posted, fetches};
}

const click = element => act(() => element.click());
const sessionRows = root => [...root.querySelectorAll('[data-session]')].map(row => row.dataset.session);
const title = (root, id) => root.querySelector(`[data-session="${id}"] .session-item-title`);
const dots = (root, id) => [...root.querySelectorAll(`[data-session="${id}"] .status-dot`)].map(dot => dot.className.replace('status-dot status-dot-', ''));
const pick = (root, index) => act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: String(index)}})));
const toggleWorkspaceStopped = (root, id = 'w') => {
  click(root.querySelector(`[data-workspace="${id}"] .sidebar-workspace-menu`));
  pick(root, 0);
};

const toggleStopped = root => {
  click(root.querySelector('[data-session="a"] .session-item-menu'));
  pick(root, 1);
};

test.each([true, false, null])('all started sessions show and Show more adds five stopped sessions, running=%s', running => {
  const sessions = Array.from({length: 23}, (_, i) => session('s' + (22 - i), {running: running ?? i % 2 === 0}));
  const {root} = setup(sessions);
  const started = sessions.filter(s => s.running);
  const stopped = sessions.filter(s => !s.running);
  expect(sessionRows(root)).toEqual(started.map(s => s.id));
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(sessions.filter(s => s.running || stopped.slice(0, 5).includes(s)).map(s => s.id));
  for (let limit = 10; limit < stopped.length + 5; limit += 5) {
    click(root.querySelector('.sidebar-show-more'));
    const visible = sessions.filter(s => s.running || stopped.indexOf(s) < limit);
    expect(sessionRows(root)).toEqual(visible.map(s => s.id));
  }
  expect(sessionRows(root)).toEqual(sessions.map(s => s.id));
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
});

test('the selected stopped session stays visible without replacing a started session', () => {
  const sessions = Array.from({length: 8}, (_, i) => session('s' + (7 - i), {running: i === 1}));
  const {store, root} = setup(sessions);
  store.update(s => ({...s, selected: 's0'}));
  expect(sessionRows(root)).toEqual(['s6', 's0']);
  expect(title(root, 's0').getAttribute('aria-current')).toBe('true');
  expect(title(root, 's6').hasAttribute('aria-current')).toBe(false);
  expect(root.querySelector('[data-session="s0"]').classList.contains('session-item-stopped')).toBe(true);
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(['s7', 's6', 's5', 's4', 's3', 's2', 's0']);
  click(root.querySelector('.sidebar-show-more'));
  expect(sessionRows(root)).toEqual(sessions.map(s => s.id));
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
});

test('selecting a child of a stopped parent keeps the family visible', () => {
  const {store, root} = setup([session('a', {running: false, children: [session('child', {parent: 'a', running: false})]})]);
  expect(sessionRows(root)).toEqual([]);
  store.update(s => ({...s, selected: 'child'}));
  expect(sessionRows(root)).toEqual(['a', 'child']);
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
});

test('stopping and starting sessions updates the default list', () => {
  const {store, root} = setup([session('a'), session('b', {running: false})]);
  expect(sessionRows(root)).toEqual(['a']);
  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {running: false}), session('b')]}]}));
  expect(sessionRows(root)).toEqual(['b']);
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(['a', 'b']);
});

test('hiding stopped sessions resets the next view to five', () => {
  const sessions = [session('running'), ...Array.from({length: 12}, (_, i) => session('s' + i, {running: false}))];
  const {root} = setup(sessions);
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(['running', 's0', 's1', 's2', 's3', 's4']);
  click(root.querySelector('.sidebar-show-more'));
  click(root.querySelector('.sidebar-show-more'));
  expect(sessionRows(root)).toEqual(sessions.map(s => s.id));
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
  click(root.querySelector('.sidebar-workspace-menu'));
  expect(root.querySelector('vscode-context-menu').data[0].label).toBe('Hide Stopped Sessions');
  pick(root, 0);
  expect(sessionRows(root)).toEqual(['running']);
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
  click(root.querySelector('.sidebar-workspace-menu'));
  expect(root.querySelector('vscode-context-menu').data[0].label).toBe('Show Stopped Sessions');
  pick(root, 0);
  expect(sessionRows(root)).toEqual(['running', 's0', 's1', 's2', 's3', 's4']);
  expect(root.querySelector('.sidebar-show-more')).not.toBeNull();
});

test.each([0, 1, 5])('Show more is absent with %s stopped sessions', count => {
  const {root} = setup(Array.from({length: count}, (_, i) => session('s' + i, {running: false})));
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toHaveLength(count);
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
});

test('workspace stopped-session visibility and pagination are independent', () => {
  const sessions = prefix => Array.from({length: 6}, (_, i) => session(prefix + i, {running: false}));
  const {root, store} = setup(sessions('a'));
  store.update(s => ({...s, workspaces: [...s.workspaces, {id: 'other', name: 'other', path: '/other', sessions: sessions('b')}]}));
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(['a0', 'a1', 'a2', 'a3', 'a4']);
  click(root.querySelector('[data-workspace="w"] .sidebar-show-more'));
  toggleWorkspaceStopped(root, 'other');
  expect(sessionRows(root)).toEqual(['a0', 'a1', 'a2', 'a3', 'a4', 'a5', 'b0', 'b1', 'b2', 'b3', 'b4']);
  toggleWorkspaceStopped(root);
  expect(sessionRows(root)).toEqual(['b0', 'b1', 'b2', 'b3', 'b4']);
});

test('Add Workspace adds a workspace', () => {
  const {root, posted} = setup([]);
  click(root.querySelector('.sidebar-add-workspace'));
  expect(posted()).toEqual([{action: 'new'}]);
});

test('clicking a session row chooses it, and a stopped one starts', () => {
  const {store, root, posted} = setup([session('a'), session('b', {running: false})]);
  toggleWorkspaceStopped(root);
  click(title(root, 'b'));
  expect(store.get().selected).toBe('b');
  expect(posted()).toEqual([{action: 'start', id: 'b'}]);
});

function rename(root, id) {
  act(() => title(root, id).dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  return root.querySelector('vscode-textfield');
}
const key = (element, key) => act(() => element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));

test('Enter saves a changed title, Escape cancels, and leaving the field saves', () => {
  const {root, posted} = setup([session('a')]);
  let field = rename(root, 'a');
  field.value = '  Renamed  ';
  key(field, 'Enter');
  expect(posted()).toEqual([{action: 'rename', id: 'a', title: 'Renamed'}]);
  expect(root.querySelector('vscode-textfield')).toBeNull();

  field = rename(root, 'a');
  field.value = 'Ignored';
  key(field, 'Escape');
  expect(posted().length).toBe(1);

  field = rename(root, 'a');
  field.value = 'Blurred';
  act(() => field.dispatchEvent(new FocusEvent('focusout', {bubbles: true})));
  expect(posted()[1]).toEqual({action: 'rename', id: 'a', title: 'Blurred'});

  field = rename(root, 'a');
  field.value = 'Session a';
  key(field, 'Enter');
  expect(posted().length).toBe(2);
});

test('keys typed in the rename field do not reach the sidebar', () => {
  const {root} = setup([session('a')]);
  const reached = [];
  root.querySelector('.sidebar').addEventListener('keydown', e => reached.push(e.key));
  const field = rename(root, 'a');
  key(field, ' ');
  key(field, 'ArrowDown');
  expect(reached).toEqual([]);
});

const choose = (root, selector, index) => {
  act(() => root.querySelector(selector).dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: String(index)}})));
};

test('a session\'s menu stops a running session and starts a stopped or failed one', () => {
  const {root, posted} = setup([
    session('running'),
    session('stopped', {running: false}),
    session('failed', {activity: 'exited'}),
  ]);
  toggleWorkspaceStopped(root);
  choose(root, '[data-session="running"]', 0);
  choose(root, '[data-session="stopped"]', 0);
  choose(root, '[data-session="failed"]', 0);
  expect(posted()).toEqual([
    {action: 'stop', id: 'running'},
    {action: 'start', id: 'stopped'},
    {action: 'start', id: 'failed'},
  ]);
  expect(root.querySelector('vscode-context-menu')).toBeNull();
});

const dialog = root => root.querySelector('dialog');
const press = (root, key) => act(() => dialog(root).querySelector('vscode-button').dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
const dialogButton = (root, label) => [...dialog(root).querySelectorAll('vscode-button')].find(b => b.textContent === label);

test('deleting a session asks first and posts only on Delete', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-session="a"]', 1);
  expect(dialog(root).open).toBe(true);
  expect(document.activeElement).toBe(dialogButton(root, 'Delete'));
  expect(document.getElementById(dialog(root).getAttribute('aria-labelledby')).textContent).toBe("Are you sure you want to delete 'Session a'?");
  expect(document.getElementById(dialog(root).getAttribute('aria-describedby')).textContent).toContain('Its agent stops');
  expect(posted()).toEqual([]);
  act(() => dialogButton(root, 'Cancel').click());
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-session="a"]', 1);
  act(() => dialogButton(root, 'Delete').click());
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([{action: 'delete', id: 'a'}]);
});

test('Escape cancels deleting a workspace', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-workspace="w"]', 1);
  expect(dialog(root).querySelector('.confirm-dialog-message').textContent).toBe('Are you sure you want to delete \'repo\'?');
  press(root, 'Escape');
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-workspace="w"]', 1);
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'w'}]);
});

test('New Session opens the unsent session at once, replacing another, without fetching profiles', () => {
  const {store, root, fetches} = setup([session('a')]);
  store.update(s => ({...s, selected: 'a'}));
  click(root.querySelector('[title="New Session"]'));
  expect(store.get().unsent).toMatchObject({workspace: 'w', profile: null, sending: false});
  expect(store.get().selected).toBeNull();
  expect(fetches).not.toHaveBeenCalled();
  expect(root.querySelector('.context-menu')).toBeNull();
});

test('nested sessions stay in creation order and ended ones dim', () => {
  const children = [session('second', {parent: 'a'}), session('first', {parent: 'a', running: false, ended: true})];
  const {root} = setup([session('a', {children}), session('p0')]);
  toggleStopped(root);
  expect(sessionRows(root)).toEqual(['a', 'second', 'first', 'p0']);
  expect(root.querySelector('[data-session="first"]').classList.contains('session-item-stopped')).toBe(true);
});

test('clicking a parent selects it and keeps its children visible', () => {
  const {store, root, posted} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  click(title(root, 'a'));
  expect(store.get().selected).toBe('a');
  expect(sessionRows(root)).toEqual(['a', 'child']);
  expect(posted()).toEqual([]);
});

test('started children stay visible and stopped children are hidden by default', () => {
  const nested = (id, extra) => session(id, {parent: 'a', ...extra});
  const {root} = setup([session('a', {children: [
    nested('one', {unseen: true}), nested('asks', {attention: true}), nested('stopped', {running: false}),
  ]})]);
  expect(sessionRows(root)).toEqual(['a', 'one', 'asks']);
});

test('a parent\'s menu shows and hides its stopped nested sessions', () => {
  const nested = (id, extra) => session(id, {parent: 'a', ...extra});
  const stopped = {running: false, ended: true};
  const {store, root} = setup([session('a', {children: [nested('one', stopped), nested('run'), nested('two', stopped)]})]);
  expect(sessionRows(root)).toEqual(['a', 'run']);

  toggleStopped(root);
  expect(sessionRows(root)).toEqual(['a', 'one', 'run', 'two']);

  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {children: [nested('one', stopped), nested('run'), nested('three', stopped)]})]}]}));
  expect(sessionRows(root)).toEqual(['a', 'one', 'run', 'three']);

  toggleStopped(root);
  expect(sessionRows(root)).toEqual(['a', 'run']);
});

test('the selected stopped session stays visible while stopped sessions are hidden', () => {
  const stopped = {running: false, ended: true};
  const {store, root} = setup([session('a', {children: [session('one', {parent: 'a', ...stopped}), session('two', {parent: 'a', ...stopped})]})]);
  store.update(s => ({...s, selected: 'two'}));
  expect(sessionRows(root)).toEqual(['a', 'two']);
});

test('each row shows only its own state', () => {
  const {store, root} = setup([session('a', {children: [
    session('asks', {parent: 'a', attention: true, activity: 'busy'}),
    session('replied', {parent: 'a', unseen: true}),
  ]})]);
  expect(dots(root, 'a')).toEqual([]);
  expect(dots(root, 'asks')).toEqual(['waiting']);
  expect(dots(root, 'replied')).toEqual(['unseen']);
  store.update(s => ({...s, selected: 'replied'}));
  expect(dots(root, 'replied')).toEqual([]);
});

test.each([
  ['idle', 'idle', []],
  ['idle', 'busy', ['child']],
  ['busy', 'idle', ['a']],
  ['busy', 'busy', ['a', 'child']],
])('each row spins for its own activity: parent=%s, child=%s', (parent, child, expected) => {
  const {root} = setup([session('a', {activity: parent, children: [session('child', {parent: 'a', activity: child})]})]);
  const busy = [...root.querySelectorAll('[name="loading"]')].map(icon => icon.closest('[data-session]').dataset.session);
  expect(busy).toEqual(expected);
});

test('waiting and unseen indicators take priority over spinners', () => {
  const {root} = setup([session('a', {activity: 'busy', unseen: true, children: [
    session('child', {parent: 'a', activity: 'busy', attention: true}),
  ]})]);
  expect(root.querySelector('[name="loading"]')).toBeNull();
  expect(dots(root, 'a')).toEqual(['unseen']);
  expect(dots(root, 'child')).toEqual(['waiting']);
});

test('a row\'s More Actions button opens its menu, and the workspace\'s opens the workspace menu', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'})]})]);
  expect(root.querySelector('[data-session="active"] .session-item-menu')).toBeNull();
  click(root.querySelector('[data-session="a"] .session-item-menu'));
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: '0'}})));
  expect(posted()).toEqual([{action: 'stop', id: 'a'}]);
  click(root.querySelector('.sidebar-workspace-menu'));
  pick(root, 1);
  expect(dialog(root).querySelector('.confirm-dialog-message').textContent).toBe('Are you sure you want to delete \'repo\'?');
});

test('Escape closes a menu and returns the focus to its button', async () => {
  const {root} = setup([session('a')]);
  const button = root.querySelector('[data-session="a"] .session-item-menu');
  button.focus();
  click(button);
  const menu = root.querySelector('vscode-context-menu');
  await menu.updateComplete;
  expect(document.activeElement).toBe(menu);
  act(() => menu.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true})));
  expect(root.querySelector('vscode-context-menu')).toBeNull();
  expect(document.activeElement).toBe(button);
});

test('active nested sessions have no menu and ended ones offer only Delete Session', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'}), session('ended', {parent: 'a', ended: true, running: false})]})]);
  act(() => root.querySelector('[data-session="active"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  expect(root.querySelector('vscode-context-menu')).toBeNull();
  toggleStopped(root);
  choose(root, '[data-session="ended"]', 0);
  expect(dialog(root).querySelector('.confirm-dialog-detail').textContent).toBe('Its chat is deleted.');
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'ended'}]);
});
