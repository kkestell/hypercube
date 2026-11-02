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

const select = (root, item) => act(() => root.querySelector('vscode-tree').dispatchEvent(new CustomEvent('vsc-tree-select', {detail: [item]})));
const sessionRows = root => [...root.querySelectorAll('[data-session]')].map(row => row.dataset.session);

test('a workspace lists five sessions, and Show more adds five', () => {
  const {root} = setup(Array.from({length: 12}, (_, i) => session('s' + i)));
  expect(sessionRows(root).length).toBe(5);
  select(root, root.querySelector('[data-show-more]'));
  expect(sessionRows(root).length).toBe(10);
  select(root, root.querySelector('[data-show-more]'));
  expect(sessionRows(root).length).toBe(12);
  expect(root.querySelector('[data-show-more]')).toBeNull();
});

test('a workspace collapses when its item closes itself', async () => {
  const {store, root} = setup([session('a')]);
  const workspace = root.querySelector('[data-key="w"]');
  expect(workspace.hasAttribute('open')).toBe(true);
  workspace.removeAttribute('open');
  await act(async () => {});
  store.update(s => ({...s, workspaces: [...s.workspaces]}));
  expect(workspace.hasAttribute('open')).toBe(false);
});

test('selecting a session row chooses it, and a stopped one starts', () => {
  const {store, root, posted} = setup([session('a'), session('b', {running: false})]);
  select(root, root.querySelector('[data-session="b"]'));
  expect(store.get().selected).toBe('b');
  expect(posted()).toEqual([{action: 'start', id: 'b'}]);
});

function rename(root, id) {
  act(() => root.querySelector(`[data-session="${id}"]`).dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
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

test('keys typed in the rename field do not reach the tree', () => {
  const {root} = setup([session('a')]);
  const reached = [];
  root.querySelector('vscode-tree').addEventListener('keydown', e => reached.push(e.key));
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
  choose(root, '[data-key="w"]', 0);
  expect(dialog(root).querySelector('.confirm-dialog-message').textContent).toBe('Are you sure you want to delete \'repo\'?');
  press(root, 'Escape');
  expect(dialog(root)).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-key="w"]', 0);
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

test('children stay in creation order, ended children dim, and Show more counts parents', () => {
  const children = [session('second', {parent: 'a'}), session('first', {parent: 'a', running: false, ended: true})];
  const {root} = setup([session('a', {children}), ...Array.from({length: 5}, (_, i) => session('p' + i))]);
  expect(sessionRows(root)).toEqual(['a', 'second', 'first', 'p0', 'p1', 'p2', 'p3']);
  expect(root.querySelector('[data-session="first"]').classList.contains('session-item-stopped')).toBe(true);
  expect(root.querySelector('[data-session="a"]').hasAttribute('open')).toBe(true);
});

test('parent clicks select without collapsing and chevron clicks collapse without selecting', async () => {
  const {store, root, posted} = setup([session('a', {children: [session('child', {parent: 'a'})]})]);
  const row = root.querySelector('[data-session="a"]');
  act(() => row.querySelector('.session-item-title').click());
  expect(store.get().selected).toBe('a');
  expect(row.hasAttribute('open')).toBe(true);
  store.update(s => ({...s, selected: null}));
  const shadow = row.attachShadow({mode: 'open'});
  const arrow = document.createElement('span');
  arrow.setAttribute('part', 'arrow-icon-container');
  shadow.append(arrow);
  act(() => arrow.dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true})));
  await act(async () => {});
  expect(row.hasAttribute('open')).toBe(false);
  expect(store.get().selected).toBeNull();
  expect(posted()).toEqual([]);
  // A child click is handled by the tree, without the parent's capture handler.
  const child = root.querySelector('[data-session="child"]');
  select(root, child);
  expect(store.get().selected).toBe('child');
  expect(row.hasAttribute('open')).toBe(false);
});

test('a running child request marks its parent while its spinner stays on its own row', () => {
  const {root} = setup([session('a', {children: [session('child', {parent: 'a', attention: true, activity: 'busy'})]})]);
  const parent = root.querySelector('[data-session="a"]');
  const decorations = parent.querySelector(':scope > .session-item-indicators');
  expect(decorations.querySelector('[title="Waiting on you: Session child"]')).not.toBeNull();
  expect(decorations.querySelector('[name="loading"]')).toBeNull();
  expect(root.querySelector('[data-session="child"] [name="loading"]')).not.toBeNull();
});

test('active children have no menu and ended children offer only Delete Session', () => {
  const {root, posted} = setup([session('a', {children: [session('active', {parent: 'a'}), session('ended', {parent: 'a', ended: true, running: false})]})]);
  act(() => root.querySelector('[data-session="active"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  expect(root.querySelector('vscode-context-menu')).toBeNull();
  choose(root, '[data-session="ended"]', 0);
  expect(dialog(root).querySelector('.confirm-dialog-detail').textContent).toBe('Its chat is deleted.');
  act(() => dialogButton(root, 'Delete').click());
  expect(posted()).toEqual([{action: 'delete', id: 'ended'}]);
});
