import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptySession, emptyEditor} from '../sessions/selection.ts';
import type {EditorView} from '../editor/types.ts';
import type {State} from '../state.ts';
import {createStore, type Store} from '../store.ts';
import {closeTab, diffTabId, fileTabId, keepOpen, openDiffTab, openFileTab, openRequestedFile, rememberScroll, resizeSplit, tabEntry} from './editor.ts';
import {moveTab, openTab, panes} from './tab-layout.ts';
import type {LineRange} from '../files/types.ts';
import type {ScrollMemory, TabId} from './types.ts';

afterEach(() => mock.restore());

const attached = (state: State): EditorView => state.editor ?? expect.unreachable('no attached session');
const openTestTab = (state: State, id: TabId): State => ({...state, editor: {...attached(state), tabs: openTab(attached(state).tabs, id)}});
const store = () => createStore(testState({session: emptySession('s'), editor: emptyEditor({session: 's'})}));
const session = (s: Store) => attached(s.get());
const tabsOf = (s: Store) => panes(session(s).tabs.root).map(pane => pane.tabs);
const activeTab = (s: Store) => panes(session(s).tabs.root)[0].active;

test('a clicked file opens in the file tabs and replaces the preview tab until it is kept open', () => {
  stubFetch(() => new Response('text'));
  const s = store();
  expect(tabsOf(s)).toEqual([]);
  openFileTab(s, 'a.go');
  openFileTab(s, 'b.go');
  expect(tabsOf(s)).toEqual([[fileTabId('b.go')]]);
  expect(session(s).tabs.preview).toBe(fileTabId('b.go'));
  expect(Object.keys(session(s).files.open)).toEqual(['b.go']);
  s.update(state => keepOpen(state, fileTabId('b.go')));
  openFileTab(s, 'c.go');
  expect(tabsOf(s)).toEqual([[fileTabId('b.go'), fileTabId('c.go')]]);
  openFileTab(s, 'b.go');
  expect(activeTab(s)).toBe(fileTabId('b.go'));
  expect(session(s).tabs.preview).toBe(fileTabId('c.go'));
});

test('resizeSplit keeps the ratio between 15 and 85', () => {
  const s = store();
  s.update(state => openTestTab(openTestTab(state, 'file:one'), 'file:two'));
  s.update(state => ({...state, editor: {...attached(state), tabs: moveTab(attached(state).tabs, 'file:two', 1, 'right')}}));
  const root = () => session(s).tabs.root ?? expect.unreachable('no tabs');
  const split = root().id;
  s.update(state => resizeSplit(state, split, 5));
  expect(root()).toMatchObject({ratio: 15});
  s.update(state => resizeSplit(state, split, 99));
  expect(root()).toMatchObject({ratio: 85});
  s.update(state => resizeSplit(state, split, 40));
  expect(root()).toMatchObject({ratio: 40});
});

test('a closed or replaced tab\'s scroll is forgotten, even when its view saves it on unmounting', () => {
  stubFetch(() => new Response('text'));
  const scroll: ScrollMemory = {top: 5, left: 0, following: false};
  const s = store();
  s.update(state => rememberScroll(openTestTab(state, 'file:one'), 's', 'file:one', scroll));
  expect(session(s).scroll).toEqual({'file:one': scroll});
  s.update(state => closeTab(state, 'file:one'));
  s.update(state => rememberScroll(state, 's', 'file:one', scroll));
  expect(session(s).scroll).toEqual({});

  openFileTab(s, 'a.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  openFileTab(s, 'b.go');
  s.update(state => rememberScroll(state, 's', fileTabId('a.go'), scroll));
  expect(session(s).scroll).toEqual({});
});

test('scroll belongs to the attached session and its open files', () => {
  const scroll: ScrollMemory = {top: 5, left: 0, following: false};
  let state = testState({
    selected: 's',
    workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's'}), testSession({id: 'other'})]})],
    editor: emptyEditor({session: 's'}),
  });
  state = rememberScroll(state, 's', 's', scroll);
  expect(attached(state).scroll).toEqual({s: scroll});
  expect(rememberScroll(state, 's', 'other', scroll)).toBe(state);
  expect(rememberScroll(state, 'other', 's', scroll)).toBe(state);
});

test('an agent opens a kept, selected file as a tab beside the chat and updates its highlights', () => {
  stubFetch(() => new Response('# Title\ntext'));
  const s = store();
  const request = (lines: LineRange[]) => openRequestedFile(s, {session: 's', path: 'guide.md', lines});
  const file = () => session(s).files.open['guide.md'];
  request([]);
  expect(tabsOf(s)).toEqual([[fileTabId('guide.md')]]);
  expect(activeTab(s)).toBe(fileTabId('guide.md'));
  expect(session(s).tabs.preview).toBeNull();
  expect(file().source).toBe(false);
  request([{start: 2, end: 2}]);
  expect(file()).toMatchObject({source: true, lines: [{start: 2, end: 2}], reveal: true});
  request([{start: 1, end: 1}]);
  expect(file().lines).toEqual([{start: 1, end: 1}]);
  request([]);
  expect(file()).toMatchObject({source: false, lines: [], reveal: false});
});

test('an agent keeps an existing preview tab without replacing another preview', () => {
  stubFetch(() => new Response('text'));
  const s = store();
  openFileTab(s, 'a.go');
  openRequestedFile(s, {session: 's', path: 'a.go', lines: []});
  expect(session(s).tabs.preview).toBeNull();
  openFileTab(s, 'b.go');
  openRequestedFile(s, {session: 's', path: 'a.go', lines: []});
  expect(tabsOf(s)).toEqual([[fileTabId('a.go'), fileTabId('b.go')]]);
  expect(session(s).tabs.preview).toBe(fileTabId('b.go'));
});

test('diff tabs share preview replacement and forget their data and scroll', () => {
  stubFetch(() => new Response('patch'));
  const s = store();
  const id = diffTabId('foo.py');
  openDiffTab(s, 'foo.py');
  expect(tabsOf(s)).toEqual([[id]]);
  expect(session(s).tabs.preview).toBe(id);
  expect(tabEntry(s.get(), id)).toMatchObject({kind: 'diff', name: 'foo.py', label: 'foo.py (Working Tree)'});
  s.update(state => ({...state, editor: {...attached(state), changes: {branch: 'main', files: [{path: 'foo.py', status: 'added'}]}}}));
  expect(tabEntry(s.get(), id).label).toBe('foo.py (Added)');
  s.update(state => keepOpen(state, id));
  openDiffTab(s, 'other.py');
  openDiffTab(s, 'foo.py');
  expect(activeTab(s)).toBe(id);
  expect(session(s).tabs.preview).toBe(diffTabId('other.py'));
  const scroll: ScrollMemory = {top: 5, left: 3, following: false};
  s.update(state => rememberScroll(state, 's', diffTabId('other.py'), scroll));
  openFileTab(s, 'source.py');
  expect(session(s).diffs['other.py']).toBeUndefined();
  expect(session(s).scroll[diffTabId('other.py')]).toBeUndefined();
  s.update(state => closeTab(state, id));
  expect(session(s).diffs['foo.py']).toBeUndefined();
  expect(session(s).scroll[id]).toBeUndefined();
  openDiffTab(s, 'next.py');
  expect(session(s).files.open['source.py']).toBeUndefined();
});
