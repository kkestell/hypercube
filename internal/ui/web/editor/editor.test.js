import {afterEach, expect, spyOn, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {fileTabId, keepOpen, openFileTab, openWorkerTab, resizeSplit} from './editor.js';
import {moveTab, panes} from './tab-layout.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const store = () => createStore({session: emptySession('s')});
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
