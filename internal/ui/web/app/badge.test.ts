import {expect, test} from 'bun:test';
import {testBridge} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {startBadge} from './badge.ts';

test('the native badge follows the number of waiting sessions', () => {
  const posted: object[] = [];
  globalThis.hypercube = testBridge({postMessage: message => posted.push(message)});
  try {
    const store = createStore(testState());
    startBadge(store);
    store.update(state => ({...state, selected: 'other'}));
    expect(posted).toEqual([]);
    store.update(state => ({...state, loaded: true, workspaces: [testWorkspace({id: 'w', sessions: [
      testSession({id: 'a', attention: true, children: [testSession({id: 'b', attention: true}), testSession({id: 'c'})]}),
      testSession({id: 'd'}),
    ]})]}));
    store.update(state => ({...state, selected: 'other'}));
    store.update(state => ({...state, workspaces: []}));
    expect(posted).toEqual([
      {type: 'badge', count: 2},
      {type: 'badge', count: 0},
    ]);
  } finally {
    delete globalThis.hypercube;
  }
});

test('the first loaded state clears a stale badge when no sessions are waiting', () => {
  const posted: object[] = [];
  globalThis.hypercube = testBridge({postMessage: message => posted.push(message)});
  try {
    const store = createStore(testState());
    startBadge(store);
    store.update(state => ({...state, loaded: true}));
    expect(posted).toEqual([{type: 'badge', count: 0}]);
  } finally {
    delete globalThis.hypercube;
  }
});
