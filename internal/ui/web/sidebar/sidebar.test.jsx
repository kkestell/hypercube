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
const state = sessions => ({workspaces: [{id: 'w', name: 'repo', path: '/repo', sessions}], selected: null, unsent: null, session: null, confirmation: null});

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
const caret = (root, id) => root.querySelector(`[data-session="${id}"] .session-item-caret`);
const dots = (root, id) => [...root.querySelectorAll(`[data-session="${id}"] .status-dot`)].map(dot => dot.className.replace('status-dot status-dot-', ''));
const summary = root => root.querySelector('.sidebar-nested-summary');

test('a workspace lists five sessions, and Show more adds five', () => {
  const {root} = setup(Array.from({length: 12}, (_, i) => session('s' + i)));
  expect(sessionRows(root).length).toBe(5);
  click(root.querySelector('.sidebar-show-more'));
  expect(sessionRows(root).length).toBe(10);
  click(root.querySelector('.sidebar-show-more'));
  expect(sessionRows(root).length).toBe(12);
  expect(root.querySelector('.sidebar-show-more')).toBeNull();
});

test('clicking a workspace header collapses and expands it, and New Session leaves it open', async () => {
  const {root, fetches} = setup([session('a')]);
  fetches.mockImplementation(async () => Response.json([]));
  click(root.querySelector('.sidebar-workspace-toggle'));
  expect(sessionRows(root)).toEqual([]);
  click(root.querySelector('.sidebar-workspace-toggle'));
  expect(sessionRows(root)).toEqual(['a']);
  await act(async () => {
    root.querySelector('[title="New Session"]').click();
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(sessionRows(root)).toEqual(['a']);
});

test('Add Workspace adds a workspace', () => {
  const {root, posted} = setup([]);
  click(root.querySelector('.sidebar-add-workspace'));
  expect(posted()).toEqual([{action: 'new'}]);
});

test('clicking a session row chooses it, and a stopped one starts', () => {
  const {store, root, posted} = setup([session('a'), session('b', {running: false})]);
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
  choose(root, '[data-workspace="w"]', 0);
  expect(dialog(root).querySelector('.confirm-dialog-message').textContent).toBe('Are you sure you want to delete \'repo\'?');
  press(root, 'Escape');
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-workspace="w"]', 0);
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'w'}]);
});

for (const cancel of [
  () => new Event('pointerdown', {bubbles: true}),
  () => new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}),
  () => new Event('blur'),
]) {
  test(`a pending lead picker is cancelled by ${cancel().type}`, async () => {
    const {root, fetches} = setup([]);
    let answer;
    fetches.mockImplementation(() => new Promise(resolve => { answer = resolve; }));
    act(() => root.querySelector('[title="New Session"]').click());
    act(() => dispatchEvent(cancel()));
    await act(async () => {
      answer(Response.json([{name: 'claude-lead', description: 'Claude'}]));
      await new Promise(resolve => setTimeout(resolve));
    });
    expect(root.querySelector('vscode-context-menu')).toBeNull();
  });
}

test('choosing a lead profile from the picker opens the unsent session', async () => {
  const {store, root, fetches} = setup([session('a')]);
  const profiles = [{name: 'claude-lead', description: 'Claude', harness: 'claude', model: 'opus', effort: 'high'}, {name: 'codex-lead', description: 'Codex', harness: 'codex'}];
  fetches.mockImplementation(async () => Response.json(profiles));
  await act(async () => {
    root.querySelector('[title="New Session"]').click();
    await new Promise(resolve => setTimeout(resolve));
  });
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: '1'}})));
  expect(store.get().unsent).toMatchObject({workspace: 'w', lead: 'codex-lead', harness: 'codex', sending: false});
  expect(store.get().selected).toBeNull();
  expect(root.querySelector('vscode-context-menu')).toBeNull();
});

test('nested sessions stay in creation order, ended ones dim, and Show more counts parents', () => {
  const children = [session('second', {parent: 'a'}), session('first', {parent: 'a', running: false, ended: true})];
  const {root} = setup([session('a', {children}), ...Array.from({length: 5}, (_, i) => session('p' + i))]);
  expect(sessionRows(root)).toEqual(['a', 'second', 'first', 'p0', 'p1', 'p2', 'p3']);
  expect(root.querySelector('[data-session="first"]').classList.contains('session-item-stopped')).toBe(true);
  expect(caret(root, 'a').getAttribute('aria-expanded')).toBe('true');
});

test('clicking a parent selects it and leaves it open; clicking its caret collapses it without selecting', () => {
  const {store, root, posted} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  click(title(root, 'a'));
  expect(store.get().selected).toBe('a');
  expect(sessionRows(root)).toEqual(['a', 'child']);
  store.update(s => ({...s, selected: null}));
  click(caret(root, 'a'));
  expect(sessionRows(root)).toEqual(['a']);
  expect(store.get().selected).toBeNull();
  expect(posted()).toEqual([]);
});

test('a collapsed parent lists its waiting nested sessions, then a summary of the rest', () => {
  const nested = (id, extra) => session(id, {parent: 'a', ...extra});
  const {store, root} = setup([session('a', {children: [
    nested('one', {unseen: true}), nested('asks', {attention: true}), nested('two'),
  ]})]);
  click(caret(root, 'a'));
  expect(sessionRows(root)).toEqual(['a', 'asks']);
  expect(summary(root).textContent).toBe('2 more nested sessions · 1 unread');

  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {children: [nested('one'), nested('asks', {attention: true}), nested('two')]})]}]}));
  expect(summary(root).textContent).toBe('2 more nested sessions');

  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {children: [nested('asks', {attention: true})]})]}]}));
  expect(sessionRows(root)).toEqual(['a', 'asks']);
  expect(summary(root)).toBeNull();

  store.update(s => ({...s, workspaces: [{...s.workspaces[0], sessions: [session('a', {children: [nested('asks', {attention: true}), nested('two')]})]}]}));
  click(summary(root));
  expect(sessionRows(root)).toEqual(['a', 'asks', 'two']);
  expect(summary(root)).toBeNull();
});

test('each row shows only its own state', () => {
  const {store, root} = setup([session('a', {children: [
    session('asks', {parent: 'a', attention: true, activity: 'busy'}),
    session('replied', {parent: 'a', unseen: true}),
  ]})]);
  expect(dots(root, 'a')).toEqual([]);
  expect(dots(root, 'asks')).toEqual(['waiting']);
  expect(dots(root, 'replied')).toEqual(['unseen']);
  expect(root.querySelector('[name="loading"]')).toBeNull();
  store.update(s => ({...s, selected: 'replied'}));
  expect(dots(root, 'replied')).toEqual([]);
});

test('a row\'s More Actions button opens its menu, and the workspace\'s opens the workspace menu', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'})]})]);
  expect(root.querySelector('[data-session="active"] .session-item-menu')).toBeNull();
  click(root.querySelector('[data-session="a"] .session-item-menu'));
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: '0'}})));
  expect(posted()).toEqual([{action: 'stop', id: 'a'}]);
  click(root.querySelector('.sidebar-workspace-menu'));
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: '0'}})));
  expect(dialog(root).querySelector('.confirm-dialog-message').textContent).toBe('Are you sure you want to delete \'repo\'?');
});

test('active nested sessions have no menu and ended ones offer only Delete Session', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'}), session('ended', {parent: 'a', ended: true, running: false})]})]);
  act(() => root.querySelector('[data-session="active"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  expect(root.querySelector('vscode-context-menu')).toBeNull();
  choose(root, '[data-session="ended"]', 0);
  expect(dialog(root).querySelector('.confirm-dialog-detail').textContent).toBe('Its chat is deleted.');
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'ended'}]);
});
