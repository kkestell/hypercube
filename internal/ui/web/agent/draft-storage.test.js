import {afterEach, beforeEach, expect, test} from 'bun:test';
import {createStore} from '../store.js';
import {openUnsent} from '../sessions/selection.js';
import {cleared, draftOf, typed, unsentCreated, unsentKey} from './drafts.js';
import {startDraftStorage} from './draft-storage.js';

beforeEach(() => localStorage.clear());
afterEach(() => localStorage.clear());

const fresh = () => createStore({drafts: {}, unsent: null, selected: 's', workspaces: [], errors: []});
const reload = () => {
  const store = fresh();
  startDraftStorage(store);
  return store;
};

test('reload restores text from multiple chats and request answers, without transient image reads', () => {
  const store = reload();
  for (const [key, text] of [['s', 'first prompt'], ['other', 'second prompt'], ['s/request', 'answer']]) {
    store.update(state => typed(state, key, text));
  }
  store.update(state => ({...state, drafts: {...state.drafts, s: {...state.drafts.s, images: [{mime: 'image/png', data: 'AA=='}], reading: 1}}}));
  const next = reload().get();
  expect(draftOf(next, 's')).toEqual({text: 'first prompt', images: [], reading: 0});
  expect(draftOf(next, 'other').text).toBe('second prompt');
  expect(draftOf(next, 's/request').text).toBe('answer');
});

test.each([null, 'workspace'])('a new-session prompt restores with workspace %s and its profile and worktree choice', workspace => {
  const store = reload();
  store.update(state => openUnsent(state, workspace));
  store.update(state => ({...state, unsent: {...state.unsent, profile: 'profile', worktree: true, sending: true}}));
  store.update(state => typed(state, unsentKey, 'new prompt'));
  const next = reload().get();
  expect(next.selected).toBeNull();
  expect(next.unsent).toMatchObject({workspace, profile: 'profile', worktree: true, sending: false});
  expect(draftOf(next, unsentKey).text).toBe('new prompt');
  store.update(state => unsentCreated(state, 'created'));
  expect(reload().get().unsent).toBeNull();
  expect(draftOf(reload().get(), 'created').text).toBe('new prompt');
});

test('sent or erased text is removed from saved drafts', () => {
  const store = reload();
  store.update(state => typed(state, 's', 'send this'));
  store.update(state => cleared(state, 's'));
  expect(reload().get().drafts).toEqual({});
  store.update(state => typed(state, 's', 'erase this'));
  store.update(state => typed(state, 's', ''));
  expect(reload().get().drafts).toEqual({});
});

test('storage failure keeps typing available, reports once, and retries on the next edit', () => {
  const store = reload();
  const storage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: {
    setItem() { throw new Error('Storage full'); },
  }});
  try {
    store.update(state => typed(state, 's', 'one'));
    store.update(state => typed(state, 's', 'two'));
    expect(draftOf(store.get(), 's').text).toBe('two');
    expect(store.get().errors).toHaveLength(1);
    expect(store.get().errors[0].message).toContain('will be lost on reload');
  } finally {
    Object.defineProperty(globalThis, 'localStorage', storage);
  }
  store.update(state => typed(state, 's', 'three'));
  expect(draftOf(reload().get(), 's').text).toBe('three');
});

test.each(['{', '{"drafts":{"s":42}}'])('invalid saved data reports an error without preventing startup: %s', value => {
  localStorage.setItem('hypercube.drafts', value);
  const store = reload();
  expect(store.get().drafts).toEqual({});
  expect(store.get().errors).toHaveLength(1);
});
