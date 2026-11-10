import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, findAll, mount, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import type {GitHubAccount} from '../github/types.ts';
import {RightPanel} from './right-panel.tsx';
import {selectRightTab} from './workbench.ts';

afterEach(unmountAll);

const tabs = (root: ParentNode) => findAll(root, '[role=tab]').map(tab => tab.getAttribute('aria-label'));
const panel = (accounts: GitHubAccount[]) => {
  const store = createStore(testState({selected: null, workspaces: [], accounts}));
  return {store, root: mount(store, <RightPanel />)};
};

test('signed-in accounts add a Pull Requests tab that counts what needs the user and lists every pull request', () => {
  const {store, root} = panel([{login: 'me', pullRequests: [testPullRequest({author: 'bob', requested: 'user'})], updated: 1}]);
  expect(tabs(root)).toEqual(['Files', 'Git', 'Search', 'Pull Requests, 1']);
  act(() => clickControl(find(root, '[role=tab][aria-label="Pull Requests, 1"]')));
  expect(store.get().workbench.rightTab).toBe('pullRequests');
  const list = find(root, '[aria-label="Pull requests across accounts"]');
  expect(list.closest('[role=tabpanel]')?.hasAttribute('hidden')).toBe(false);
  expect(list.textContent).toContain('To review');
});

test('without an account the Pull Requests tab is absent and Files shows in its place', () => {
  const {store, root} = panel([]);
  act(() => store.update(state => selectRightTab(state, 'pullRequests')));
  expect(tabs(root)).toEqual(['Files', 'Git', 'Search']);
  expect(find(root, '[role=tab][aria-label=Files]').getAttribute('aria-selected')).toBe('true');
  expect(root.querySelector('[aria-label="Pull requests across accounts"]')).toBeNull();
});
