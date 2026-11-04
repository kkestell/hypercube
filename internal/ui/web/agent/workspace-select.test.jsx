import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {openUnsent, unsentProfile, unsentWorktree} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {typed, unsentKey} from './drafts.js';
import {WorkspaceSelect} from './workspace-select.jsx';

afterEach(() => {
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
  choose(root, 'folder');
  expect(store.get().unsent.workspace).toBe('git');
});
