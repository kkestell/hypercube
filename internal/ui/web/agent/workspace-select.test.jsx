import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {openUnsent, receivedWorkspaces, unsentProfile, unsentWorktree} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {canSend, typed, unsentKey} from './drafts.js';
import {WorkspaceSelect} from './workspace-select.jsx';

let fetches;
afterEach(() => {
  fetches?.mockRestore();
  fetches = null;
  unmountAll();
});

const workspaces = [
  {id: 'git', name: 'repo', path: '/repo', noGit: false, sessions: []},
  {id: 'folder', name: 'notes', path: '/notes', noGit: true, sessions: []},
];
const setup = (list = workspaces) => {
  const initial = {workspaces: list, unsent: null, selected: null, session: null, drafts: {}, errors: []};
  const state = typed(unsentProfile(openUnsent(initial), {name: 'lead'}), unsentKey, 'Keep this draft');
  const store = createStore(state);
  return {store, root: mount(store, <WorkspaceSelect />)};
};
const choose = (root, value) => {
  const select = root.querySelector('vscode-single-select');
  select.value = value;
  act(() => select.dispatchEvent(new Event('change')));
};
const settle = () => act(() => new Promise(resolve => setTimeout(resolve)));

test('changing workspace keeps the draft and profile and resets the worktree choice', () => {
  const {store, root} = setup();
  store.update(s => unsentWorktree(s, true));
  const identity = store.get().unsent.identity;
  expect([...root.querySelectorAll('vscode-option')].map(option => [option.textContent, option.getAttribute('description')])).toEqual([
    ['repo', '/repo'], ['notes', '/notes'],
  ]);
  choose(root, 'folder');
  expect(store.get().unsent).toMatchObject({workspace: 'folder', profile: 'lead', worktree: false, identity});
  expect(store.get().drafts[unsentKey].text).toBe('Keep this draft');
});

test('workspace controls disable during creation and workspace changes are ignored', () => {
  const {store, root} = setup();
  store.update(s => ({...s, unsent: {...s.unsent, sending: true}}));
  expect(root.querySelector('vscode-single-select').hasAttribute('disabled')).toBe(true);
  expect(root.querySelector('vscode-button').hasAttribute('disabled')).toBe(true);
  choose(root, 'folder');
  expect(store.get().unsent.workspace).toBe('git');
});

test('a first workspace can be added and selected without losing the draft', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('added'));
  const {store, root} = setup([]);
  const session = {id: unsentKey, activity: 'idle'};
  expect(canSend(store.get(), session)).toBe(false);
  store.update(s => receivedWorkspaces(s, [{id: 'added', name: 'new', path: '/new', sessions: []}]));
  expect(store.get().unsent.workspace).toBeNull();
  act(() => root.querySelector('vscode-button').click());
  await settle();
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('action')).toBe('new');
  expect(store.get().unsent.workspace).toBe('added');
  expect(store.get().drafts[unsentKey].text).toBe('Keep this draft');
  expect(canSend(store.get(), session)).toBe(true);
});

test('cancelling the folder picker keeps the current workspace', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const {store, root} = setup();
  act(() => root.querySelector('vscode-button').click());
  await settle();
  expect(store.get().unsent.workspace).toBe('git');
});

test('a folder picker result does not change a replacement new session', async () => {
  let respond;
  fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { respond = resolve; }));
  const {store, root} = setup();
  act(() => root.querySelector('vscode-button').click());
  store.update(s => openUnsent(s, 'folder'));
  respond(new Response('added'));
  await settle();
  expect(store.get().unsent.workspace).toBe('folder');
});
