import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {Sidebar} from './sidebar.jsx';

let spies = [];
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  unmountAll();
});

const lead = (activity = 'idle') => ({id: 'lead', name: 'lead', role: 'lead', activity});
const session = (id, extra = {}) => ({id, title: 'Session ' + id, running: true, unseen: false, agents: [lead()], ...extra});
const state = sessions => ({workspaces: [{id: 'w', name: 'repo', path: '/repo', sessions}], selected: null, unsent: null, session: null});

function setup(sessions) {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const store = createStore(state(sessions));
  const root = mount(store, <Sidebar />);
  const posted = () => fetches.mock.calls.map(([url]) => Object.fromEntries(new URL(url, 'http://x/').searchParams));
  return {store, root, posted};
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

test('a session\'s menu stops a running session and starts a stopped or failed one', () => {
  spies.push(spyOn(globalThis, 'confirm').mockImplementation(() => true));
  const {root, posted} = setup([
    session('running'),
    session('stopped', {running: false}),
    session('failed', {agents: [lead('exited')]}),
  ]);
  const choose = (id, index) => {
    act(() => root.querySelector(`[data-session="${id}"]`).dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
    act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: String(index)}})));
  };
  choose('running', 0);
  choose('stopped', 0);
  choose('failed', 0);
  choose('running', 1);
  expect(posted()).toEqual([
    {action: 'stop', id: 'running'},
    {action: 'start', id: 'stopped'},
    {action: 'start', id: 'failed'},
    {action: 'delete', id: 'running'},
  ]);
  expect(root.querySelector('vscode-context-menu')).toBeNull();
});
