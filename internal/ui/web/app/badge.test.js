import {expect, test} from 'bun:test';
import {createStore} from '../store.js';
import {startBadge} from './badge.js';

test('the native badge follows the number of waiting sessions', () => {
  const posted = [];
  globalThis.webkit = {messageHandlers: {window: {postMessage: message => posted.push(message)}}};
  try {
    const store = createStore({workspaces: []});
    startBadge(store);
    store.update(() => ({workspaces: [{sessions: [
      {attention: true, children: [{attention: true}, {attention: false}]},
      {attention: false, children: []},
    ]}]}));
    store.update(state => ({...state, selected: 'other'}));
    store.update(() => ({workspaces: []}));
    expect(posted).toEqual([
      {type: 'badge', count: 0},
      {type: 'badge', count: 2},
      {type: 'badge', count: 0},
    ]);
  } finally {
    delete globalThis.webkit;
  }
});
