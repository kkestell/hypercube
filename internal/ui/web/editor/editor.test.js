import {afterEach, expect, spyOn, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {closeTab, fileTabId, keepOpen, openFileTab, openWorkerTab, rememberScroll, resizeSplit} from './editor.js';
import {moveTab, panes} from './tab-layout.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const store = () => createStore({workspaces: [], session: emptySession('s')});
const tabsOf = s => panes(s.get().session.tabs.root).map(pane => pane.tabs);

test('a clicked file replaces the preview tab until it is kept open', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('text'));
  const s = store();
  openFileTab(s, 'a.go');
  openFileTab(s, 'b.go');
  expect(tabsOf(s)).toEqual([[fileTabId('b.go')]]);
  expect(s.get().session.tabs.preview).toBe(fileTabId('b.go'));
  expect(Object.keys(s.get().session.files.open)).toEqual(['b.go']);
  s.update(state => keepOpen(state, fileTabId('b.go')));
  openFileTab(s, 'c.go');
  expect(tabsOf(s)).toEqual([[fileTabId('b.go'), fileTabId('c.go')]]);
  openFileTab(s, 'b.go');
  expect(s.get().session.tabs.root.active).toBe(fileTabId('b.go'));
  expect(s.get().session.tabs.preview).toBe(fileTabId('c.go'));
});

test('Open Transcript makes the worker tab active', () => {
  const s = store();
  s.update(state => openWorkerTab(openWorkerTab(state, 'one'), 'two'));
  s.update(state => openWorkerTab(state, 'one'));
  expect(s.get().session.tabs.root).toMatchObject({tabs: ['one', 'two'], active: 'one'});
});

test('resizeSplit keeps the ratio between 15 and 85', () => {
  const s = store();
  s.update(state => openWorkerTab(openWorkerTab(state, 'one'), 'two'));
  s.update(state => ({...state, session: {...state.session, tabs: moveTab(state.session.tabs, 'two', 1, 'right')}}));
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
  s.update(state => rememberScroll(openWorkerTab(state, 'one'), 's', 'one', scroll));
  expect(s.get().session.scroll).toEqual({one: scroll});
  s.update(state => closeTab(state, 'one'));
  s.update(state => rememberScroll(state, 's', 'one', scroll));
  expect(s.get().session.scroll).toEqual({});

  openFileTab(s, 'a.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  openFileTab(s, 'b.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  expect(s.get().session.scroll).toEqual({});
});

test('the lead\'s scroll is kept, though the lead is not a tab', () => {
  const scroll = {top: 5, left: 0, following: false};
  const lead = {id: 'lead', role: 'lead'};
  const s = createStore({workspaces: [{sessions: [{agents: [lead]}]}], session: emptySession('s')});
  s.update(state => rememberScroll(state, 's', 'lead', scroll));
  expect(s.get().session.scroll).toEqual({lead: scroll});
});
