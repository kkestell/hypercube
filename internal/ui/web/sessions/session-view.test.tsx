import {afterEach, expect, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.tsx';
import {testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {SessionView} from './session-view.tsx';

afterEach(unmountAll);

const view = (accounts: {login: string; pullRequests: []; updated: number}[]) =>
  mount(createStore(testState({selected: null, workspaces: [testWorkspace({id: 'w'})], accounts})), <SessionView />);

test('with nothing selected, the center stays empty, with or without an account', () => {
  for (const accounts of [[{login: 'me', pullRequests: [] as [], updated: 1}], []]) {
    const root = view(accounts);
    expect(root.querySelector('[aria-label="Pull requests across accounts"]')).toBeNull();
    expect(root.textContent).not.toContain('Checking GitHub.');
    unmountAll();
  }
});
