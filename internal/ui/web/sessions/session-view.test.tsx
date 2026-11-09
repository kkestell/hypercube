import {afterEach, expect, test} from 'bun:test';
import {find, mount, unmountAll} from '../../test-render.tsx';
import {testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {SessionView} from './session-view.tsx';

afterEach(unmountAll);

const view = (accounts: {login: string; pullRequests: []; updated: number}[]) =>
  mount(createStore(testState({selected: null, workspaces: [testWorkspace({id: 'w'})], accounts})), <SessionView />);

test('with nothing selected, the center lists pull requests once an account exists', () => {
  expect(find(view([{login: 'me', pullRequests: [], updated: 1}]), '[aria-label="Pull requests across accounts"]')).toBeTruthy();
});

test('with nothing selected and no account, the center stays empty', () => {
  const root = view([]);
  expect(root.querySelector('[aria-label="Pull requests across accounts"]')).toBeNull();
  expect(root.textContent).not.toContain('Checking GitHub.');
});
