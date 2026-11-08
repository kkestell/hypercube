import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptyEditor} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {WorkingContext} from './working-context.tsx';

afterEach(unmountAll);

test('context identifies the worktree and actual branch and opens the Git panel', () => {
  const session = testSession({id: 's', dir: '/repo/.hypercube/s', worktree: true});
  const store = createStore(testState({
    selected: 's', workspaces: [testWorkspace({id: 'w', name: 'Project', path: '/repo', sessions: [session]})],
    editor: {...emptyEditor({session: 's'}), changes: {branch: 'feature', files: []}},
  }));
  const root = mount(store, <WorkingContext />);
  expect(root.textContent).toContain('/repo/.hypercube/s');
  expect(find(root, '[title="/repo/.hypercube/s"]')).toBeTruthy();
  expect(find(root, 'button').textContent).toBe('feature');
  act(() => find(root, 'button').click());
  expect(store.get().workbench.rightPanel).toBe(true);
  expect(store.get().workbench.rightTab).toBe('git');
  act(() => store.update(s => ({...s, editor: {...s.editor!, changes: {branch: 'another-branch', files: []}}})));
  expect(find(root, 'button').textContent).toBe('another-branch');
});

test('checkout path without a branch name shows detached HEAD', () => {
  const session = testSession({id: 's', dir: '/repo'});
  const store = createStore(testState({
    selected: 's', workspaces: [testWorkspace({id: 'w', path: '/repo', sessions: [session]})],
    editor: {...emptyEditor({session: 's'}), changes: {branch: '', files: []}},
  }));
  const root = mount(store, <WorkingContext />);
  expect(root.textContent).toContain('/repo');
  expect(find(root, 'button').textContent).toBe('Detached HEAD');
});
