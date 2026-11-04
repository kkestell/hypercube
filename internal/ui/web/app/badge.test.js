import {expect, test} from 'bun:test';
import {createStore} from '../store.js';
import {startBadge} from './badge.js';

test('the native badge follows the number of waiting sessions', () => {
  const posted = [];
  globalThis.webkit = {messageHandlers: {window: {postMessage: message => posted.push(message)}}};
  try {
    const store = createStore({workspaces: [], loaded: false});
    startBadge(store);
    store.update(state => ({...state, selected: 'other'}));
    expect(posted).toEqual([]);
    store.update(() => ({loaded: true, workspaces: [{sessions: [
      {attention: true, children: [{attention: true}, {attention: false}]},
      {attention: false, children: []},
    ]}]}));
    store.update(state => ({...state, selected: 'other'}));
    store.update(() => ({loaded: true, workspaces: []}));
    expect(posted).toEqual([
      {type: 'badge', count: 2},
      {type: 'badge', count: 0},
    ]);
  } finally {
    delete globalThis.webkit;
  }
});

test('the first loaded state clears a stale badge when no sessions are waiting', () => {
  const posted = [];
  globalThis.webkit = {messageHandlers: {window: {postMessage: message => posted.push(message)}}};
  try {
    const store = createStore({workspaces: [], loaded: false});
    startBadge(store);
    store.update(state => ({...state, loaded: true}));
    expect(posted).toEqual([{type: 'badge', count: 0}]);
  } finally {
    delete globalThis.webkit;
  }
});
