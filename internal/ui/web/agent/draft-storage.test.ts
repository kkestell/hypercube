import {afterEach, beforeEach, expect, test} from 'bun:test';
import {createStore} from '../store.ts';
import {openUnsent, selectSession, unsentSettingsChosen, unsentWorktree} from '../sessions/selection.ts';
import type {Unsent} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {testConfig, testState} from '../../test-state.ts';
import {receivedConfig} from '../app/config.ts';
import {cleared, draftOf, typed, unsentCreated, unsentKey} from './drafts.ts';
import {startDraftStorage} from './draft-storage.ts';

beforeEach(() => localStorage.clear());
afterEach(() => localStorage.clear());

const fresh = () => createStore(testState({selected: 's'}));
const unsentChanged = (state: State, change: Partial<Unsent>): State => ({...state, unsent: state.unsent && {...state.unsent, ...change}});
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
  store.update(state => ({...state, drafts: {...state.drafts, s: {...state.drafts.s, images: [{mime_type: 'image/png', data: 'AA=='}], reading: 1}}}));
  const next = reload().get();
  expect(draftOf(next, 's')).toEqual({text: 'first prompt', images: [], reading: 0});
  expect(draftOf(next, 'other').text).toBe('second prompt');
  expect(draftOf(next, 's/request').text).toBe('answer');
});

test.each([null, 'workspace'])('a new-session prompt restores with workspace %s and its settings and worktree choice', workspace => {
  const store = reload();
  store.update(state => openUnsent(state, workspace));
  store.update(state => unsentChanged(state, {settings: {harness: 'claude', model: 'opus', effort: 'high'}, worktree: true, branch: {name: 'feature/login', start: 'main'}, sending: true}));
  store.update(state => typed(state, unsentKey(workspace), 'new prompt'));
  const next = reload().get();
  expect(next.selected).toBeNull();
  expect(next.unsent).toMatchObject({workspace, settings: {harness: 'claude', model: 'opus', effort: 'high'}, worktree: true, branch: {name: 'feature/login', start: 'main'}, sending: false});
  expect(draftOf(next, unsentKey(workspace)).text).toBe('new prompt');
  store.update(state => unsentCreated(state, 'created', unsentKey(workspace)));
  expect(reload().get().unsent).toBeNull();
  expect(draftOf(reload().get(), 'created').text).toBe('new prompt');
});

test('a draft saved before branch choices existed restores without a branch', () => {
  localStorage.setItem('hypercube.drafts', JSON.stringify({drafts: {[unsentKey('w')]: 'prompt'}, unsent: {workspace: 'w', settings: null, worktree: true}, unsentDrafts: {}}));
  expect(reload().get().unsent).toMatchObject({workspace: 'w', worktree: true, branch: null});
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

test('parked new-session drafts restore across workspaces without replacing the selected session', () => {
  const store = reload();
  store.update(state => typed(unsentWorktree(unsentSettingsChosen(openUnsent(state, 'a'), {harness: 'claude', model: 'opus', effort: 'high'}), true), unsentKey('a'), 'first workspace'));
  store.update(state => typed(openUnsent(state, 'b'), unsentKey('b'), 'second workspace'));
  store.update(state => selectSession(state, 's'));
  const restored = reload();
  expect(restored.get().selected).toBe('s');
  expect(restored.get().unsent).toBeNull();
  expect(Object.keys(restored.get().unsentDrafts)).toEqual([unsentKey('a'), unsentKey('b')]);
  restored.update(state => openUnsent(state, 'a'));
  expect(restored.get().unsent).toMatchObject({workspace: 'a', settings: {harness: 'claude', model: 'opus', effort: 'high'}, worktree: true, sending: false});
  expect(draftOf(restored.get(), unsentKey('a')).text).toBe('first workspace');
  restored.update(state => openUnsent(state, 'b'));
  expect(draftOf(restored.get(), unsentKey('b')).text).toBe('second workspace');
});

test('a new-session draft saved before its defaults arrived takes them once', () => {
  localStorage.setItem('hypercube.drafts', JSON.stringify({drafts: {[unsentKey('w')]: 'existing prompt', s: 'session draft'}, unsent: {workspace: 'w', settings: null, worktree: null}, unsentDrafts: {}}));
  const restored = reload();
  expect(restored.get().unsent).toMatchObject({workspace: 'w', settings: null, worktree: null});
  const opus = {harness: 'claude', model: 'opus', effort: 'high'};
  restored.update(state => receivedConfig(state, testConfig({default_selection: opus})));
  restored.update(state => receivedConfig(state, testConfig({default_selection: {harness: 'pi', model: 'sol', effort: 'low'}})));
  expect(reload().get().unsent).toMatchObject({workspace: 'w', settings: opus, worktree: null});
  expect(draftOf(restored.get(), unsentKey('w')).text).toBe('existing prompt');
  expect(draftOf(restored.get(), 's').text).toBe('session draft');
});

test('storage failure keeps typing available, reports once, and retries on the next edit', () => {
  const store = reload();
  const storage = localStorage;
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
    Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: storage});
  }
  store.update(state => typed(state, 's', 'three'));
  expect(draftOf(reload().get(), 's').text).toBe('three');
});

test.each([
  '{',
  '{"drafts":{"s":42},"unsent":null,"unsentDrafts":{}}',
  '{"drafts":{"s":"text"},"unsent":null}',
  '{"drafts":{"new/w":"text"},"unsent":{"workspace":"w","worktree":true},"unsentDrafts":{}}',
])('invalid saved data reports an error without preventing startup: %s', value => {
  localStorage.setItem('hypercube.drafts', value);
  const store = reload();
  expect(store.get().drafts).toEqual({});
  expect(store.get().errors).toHaveLength(1);
});
