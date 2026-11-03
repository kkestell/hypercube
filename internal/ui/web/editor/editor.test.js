import {afterEach, expect, spyOn, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {agentTabId, closeTab, fileTabId, keepOpen, openFileTab, openRequestedFile, rememberScroll, resizeSplit} from './editor.js';
import {moveTab, openTab, panes} from './tab-layout.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const openTestTab = (state, id) => ({...state, session: {...state.session, tabs: openTab(state.session.tabs, id)}});
const store = () => createStore({workspaces: [], session: emptySession('s')});
const tabsOf = s => panes(s.get().session.tabs.root).map(pane => pane.tabs);

test('a clicked file opens beside the agent and replaces the preview tab until it is kept open', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('text'));
  const s = store();
  expect(tabsOf(s)).toEqual([[agentTabId]]);
  openFileTab(s, 'a.go');
  openFileTab(s, 'b.go');
  expect(tabsOf(s)).toEqual([[agentTabId], [fileTabId('b.go')]]);
  expect(s.get().session.tabs.preview).toBe(fileTabId('b.go'));
  expect(Object.keys(s.get().session.files.open)).toEqual(['b.go']);
  s.update(state => keepOpen(state, fileTabId('b.go')));
  openFileTab(s, 'c.go');
  expect(tabsOf(s)).toEqual([[agentTabId], [fileTabId('b.go'), fileTabId('c.go')]]);
  openFileTab(s, 'b.go');
  expect(panes(s.get().session.tabs.root)[1].active).toBe(fileTabId('b.go'));
  expect(s.get().session.tabs.preview).toBe(fileTabId('c.go'));
});

test('resizeSplit keeps the ratio between 15 and 85', () => {
  const s = store();
  s.update(state => openTestTab(openTestTab(state, 'file:one'), 'file:two'));
  s.update(state => ({...state, session: {...state.session, tabs: moveTab(state.session.tabs, 'file:two', 1, 'right')}}));
  const split = s.get().session.tabs.root.id;
  s.update(state => resizeSplit(state, split, 5));
  expect(s.get().session.tabs.root.ratio).toBe(15);
  s.update(state => resizeSplit(state, split, 99));
  expect(s.get().session.tabs.root.ratio).toBe(85);
  s.update(state => resizeSplit(state, split, 40));
  expect(s.get().session.tabs.root.ratio).toBe(40);
});

test('a closed or replaced tab\'s scroll is forgotten, even when its view saves it on unmounting', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('text'));
  const scroll = {top: 5, left: 0, following: false};
  const s = store();
  s.update(state => rememberScroll(openTestTab(state, 'file:one'), 's', 'file:one', scroll));
  expect(s.get().session.scroll).toEqual({'file:one': scroll});
  s.update(state => closeTab(state, 'file:one'));
  s.update(state => rememberScroll(state, 's', 'file:one', scroll));
  expect(s.get().session.scroll).toEqual({});

  openFileTab(s, 'a.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  openFileTab(s, 'b.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  expect(s.get().session.scroll).toEqual({});
});

test('scroll belongs to the attached session and its open files', () => {
  const scroll = {top: 5, left: 0, following: false};
  let state = {selected: 's', workspaces: [{sessions: [{id: 's', children: []}, {id: 'other', children: []}]}], session: emptySession('s')};
  state = rememberScroll(state, 's', 's', scroll);
  expect(state.session.scroll).toEqual({s: scroll});
  expect(rememberScroll(state, 's', 'other', scroll)).toBe(state);
  expect(rememberScroll(state, 'other', 's', scroll)).toBe(state);
});

test('an agent opens a kept, selected file beside the chat and updates its highlights', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('# Title\ntext'));
  const s = store();
  const request = lines => openRequestedFile(s, {session: 's', path: 'guide.md', lines});
  request([]);
  expect(tabsOf(s)).toEqual([[agentTabId], [fileTabId('guide.md')]]);
  expect(panes(s.get().session.tabs.root)[1].active).toBe(fileTabId('guide.md'));
  expect(s.get().session.tabs.preview).toBeNull();
  expect(s.get().session.files.open['guide.md'].source).toBe(false);
  request([{start: 2, end: 2}]);
  expect(s.get().session.files.open['guide.md']).toMatchObject({source: true, lines: [{start: 2, end: 2}], reveal: true});
  request([{start: 1, end: 1}]);
  expect(s.get().session.files.open['guide.md'].lines).toEqual([{start: 1, end: 1}]);
  request([]);
  expect(s.get().session.files.open['guide.md']).toMatchObject({source: false, lines: [], reveal: false});
});

test('an agent keeps an existing preview tab without replacing another preview', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('text'));
  const s = store();
  openFileTab(s, 'a.go');
  openRequestedFile(s, {session: 's', path: 'a.go', lines: []});
  expect(s.get().session.tabs.preview).toBeNull();
  openFileTab(s, 'b.go');
  openRequestedFile(s, {session: 's', path: 'a.go', lines: []});
  expect(tabsOf(s)).toEqual([[agentTabId], [fileTabId('a.go'), fileTabId('b.go')]]);
  expect(s.get().session.tabs.preview).toBe(fileTabId('b.go'));
});
