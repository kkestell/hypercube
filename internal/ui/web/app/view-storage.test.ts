import {afterEach, beforeEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {startDraftStorage} from '../agent/draft-storage.ts';
import {draftOf, typed, unsentKey} from '../agent/drafts.ts';
import {activatePane, keepOpen, mainTabs, moveTab, openDiffTab, openFileAt, openRequestedFile, rememberScroll, resizeSplit} from '../editor/editor.ts';
import {mainTabId, panes} from '../editor/tab-layout.ts';
import {openUnsent, receivedWorkspaces, selectSession, selectWorkspace} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {receivedTranscript, setExpanded} from '../transcript/transcripts.ts';
import {startViewStorage} from './view-storage.ts';
import {hideRegion, resizeRegion, selectRightTab, toggle, toggleWrap} from './workbench.ts';

const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})]})];
const fresh = (selected = '') => createStore(testState({selected}));

beforeEach(() => localStorage.clear());
afterEach(() => { mock.restore(); localStorage.clear(); });

test.each(['files', 'git', 'search'] as const)('the %s panel choice survives reload without dropping session or workspace views', rightTab => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  openFileAt(store, 'session.md', []);
  store.update(state => selectWorkspace(state, 'w'));
  openFileAt(store, 'workspace.md', []);
  store.update(state => selectRightTab(state, rightTab));

  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().errors).toEqual([]);
  expect(reloaded.get().workbench.rightTab).toBe(rightTab);
  expect(panes(reloaded.get().editor!.tabs.root)[0].tabs).toContain('file:workspace.md');
  reloaded.update(state => selectSession(state, 'a'));
  expect(panes(reloaded.get().editor!.tabs.root)[0].tabs).toEqual([mainTabId, 'file:session.md']);
});

test('session switches and reloads preserve independent view choices without caching server data', async () => {
  stubFetch(() => new Response('file contents must not be saved'));
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  store.update(state => typed(state, 'a', 'unfinished prompt'));
  openRequestedFile(store, {session: 'a', path: 'guide.md', lines: [{start: 2}]});
  openDiffTab(store, 'changed.go');
  store.update(state => keepOpen(state, 'diff:changed.go'));
  openDiffTab(store, 'changed.go', 'branch');
  store.update(state => keepOpen(state, 'branch-diff:changed.go'));
  store.update(state => moveTab(state, 'file:guide.md', 1, 'right'));
  const split = store.get().editor!.tabs.root!.id;
  store.update(state => resizeSplit(state, split, 37));
  store.update(state => activatePane(state, panes(state.editor!.tabs.root)[1].id));
  store.update(state => setExpanded(state, 'activity', true));
  store.update(state => ({...state, editor: {...state.editor!, files: {...state.editor!.files, expanded: {src: true}}}}));
  store.update(state => rememberScroll(state, 'a', 'file:guide.md', {top: 120, left: 8, following: false}));
  store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: true, items: [{id: 'message', seq: 1, kind: 'text', text: 'transcript must not be saved'}]}));
  await Promise.resolve();
  const layout = store.get().editor!.tabs;
  const activePane = store.get().editor!.activePane;

  store.update(state => selectSession(state, 'b'));
  expect(store.get().sessionViews.a).toBeDefined();
  expect(store.get().sessionViews.b).toBeUndefined();
  expect(store.get().editor!.tabs).toEqual(mainTabs);
  store.update(state => rememberScroll(state, 'a', 'a', {top: 345, left: 0, following: false}));
  store.update(state => selectSession(state, 'a'));
  expect(store.get().sessionViews.a).toBeUndefined();
  expect(store.get().editor!.tabs).toEqual(layout);
  expect(store.get().editor!.activePane).toBe(activePane);
  expect(store.get().editor!.scroll.a.top).toBe(345);
  expect(draftOf(store.get(), 'a').text).toBe('unfinished prompt');

  const text = localStorage.getItem('hypercube.views')!;
  expect(text).not.toContain('contents must not');
  expect(text).not.toContain('transcript must not');
  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  const restored = reloaded.get().editor!;
  expect(reloaded.get().selected).toBe('a');
  expect(restored.tabs).toEqual(layout);
  expect(restored.activePane).toBe(activePane);
  expect(restored.files.expanded).toEqual({src: true});
  expect(reloaded.get().session!.expanded.activity).toBe(true);
  expect(restored.scroll['file:guide.md']).toEqual({top: 120, left: 8, following: false});
  expect(restored.files.open['guide.md']).toMatchObject({text: null, etag: null, source: true, lines: [{start: 2}], reveal: false});
  expect(restored.diffs['diff:changed.go']).toEqual({text: null, message: null});
  expect(restored.diffs['branch-diff:changed.go']).toEqual({text: null, message: null});
  expect(restored.gitComparison).toBe('uncommitted');
  expect(restored.branchChanges).toBeNull();
  expect(reloaded.get().session!.transcript.order).toEqual([]);
  expect(reloaded.get().session!.transcriptReady).toBe(false);
});

test('terminal visibility belongs to each session, survives reload, and stays closed for drafts', () => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  store.update(state => toggle(state, 'panel'));
  expect(store.get().editor?.panel).toBe(true);
  store.update(state => selectSession(state, 'b'));
  expect(store.get().editor?.panel).toBe(false);
  store.update(state => selectSession(state, 'a'));
  expect(store.get().editor?.panel).toBe(true);
  store.update(state => openUnsent(state, 'w'));
  expect(store.get().session).toBeNull();
  const draft = store.get();
  store.update(state => toggle(state, 'panel'));
  expect(store.get()).toBe(draft);
  store.update(state => selectSession(state, 'a'));
  expect(store.get().editor?.panel).toBe(true);

  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().editor?.panel).toBe(true);
  reloaded.update(state => selectSession(state, 'b'));
  expect(reloaded.get().editor?.panel).toBe(false);
});

test('an explicit session URL wins over saved selection and a restored new-session draft', () => {
  const previous = fresh('a');
  startViewStorage(previous);
  previous.update(state => receivedWorkspaces(state, workspaces));
  localStorage.setItem('hypercube.drafts', JSON.stringify({drafts: {[unsentKey('w')]: 'keep this draft'}, unsent: {workspace: 'w', settings: null, worktree: true}, unsentDrafts: {}}));
  const explicit = fresh('b');
  startViewStorage(explicit);
  startDraftStorage(explicit);
  explicit.update(state => selectSession(state, 'b'));
  explicit.update(state => receivedWorkspaces(state, workspaces));
  expect(explicit.get().selected).toBe('b');
  expect(explicit.get().session!.id).toBe('b');
  expect(explicit.get().unsent).toBeNull();
  expect(explicit.get().unsentDrafts[unsentKey('w')]).toMatchObject({workspace: 'w', worktree: true});
  expect(draftOf(explicit.get(), unsentKey('w')).text).toBe('keep this draft');
});

test('removed sessions are pruned and late cleanup cannot recreate their view', () => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  store.update(state => selectSession(state, 'b'));
  store.update(state => selectSession(state, 'a'));
  store.update(state => receivedWorkspaces(state, [testWorkspace({id: 'w', sessions: [testSession({id: 'b'})]})]));
  const before = store.get();
  store.update(state => rememberScroll(state, 'a', 'a', {top: 500, left: 0, following: false}));
  expect(store.get()).toBe(before);
  expect(store.get().sessionViews.a).toBeUndefined();
  expect(JSON.parse(localStorage.getItem('hypercube.views')!).sessionViews.a).toBeUndefined();
});

test('unavailable storage keeps in-memory changes usable', () => {
  const store = fresh();
  startViewStorage(store);
  const wrap = store.get().workbench.wrap;
  const storage = localStorage;
  Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: {setItem() { throw new Error('quota exceeded'); }}});
  try {
    store.update(toggleWrap);
    store.update(toggleWrap);
    store.update(toggleWrap);
    expect(store.get().workbench.wrap).toBe(!wrap);
    expect(store.get().errors).toHaveLength(1);
    expect(store.get().errors[0].message).toContain('View state could not be saved');
  } finally {
    Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: storage});
  }
  store.update(toggleWrap);
  expect(JSON.parse(localStorage.getItem('hypercube.views')!).workbench.wrap).toBe(wrap);
});

test('invalid saved layouts report their failure and retain defaults', () => {
  localStorage.setItem('hypercube.views', '{"selected":"a","workbench":{},"sessionViews":{}}');
  const store = fresh();
  startViewStorage(store);
  expect(store.get().sessionViews).toEqual({});
  expect(store.get().workbench.sizes.sidebar).toBe(290);
  expect(store.get().errors[0].message).toContain('Could not restore view state');
});

test('saved views missing workspace views report their failure and retain defaults', () => {
  const previous = fresh('a');
  startViewStorage(previous);
  previous.update(state => receivedWorkspaces(state, workspaces));
  previous.update(state => resizeRegion(state, 'sidebar', 400));
  const saved = JSON.parse(localStorage.getItem('hypercube.views')!);
  delete saved.workspaceViews;
  localStorage.setItem('hypercube.views', JSON.stringify(saved));

  const store = fresh();
  startViewStorage(store);
  expect(store.get().sessionViews).toEqual({});
  expect(store.get().workbench.sizes.sidebar).toBe(290);
  expect(store.get().errors[0].message).toContain('Could not restore view state');
});

test('layouts saved with chat and dashboards outside the tabs gain the main tab at their left', () => {
  const files = {id: 1, tabs: ['file:a.md'], active: 'file:a.md'};
  const view = {activePane: 1, panel: false, scroll: {}, folders: {}, files: {}};
  const saved = {
    selected: 'a',
    workbench: {...fresh().get().workbench, sizes: {sidebar: 290, rightPanel: 320, panel: 240, editor: 560}},
    sessionViews: {b: {...view, expanded: {}, tabs: {root: files, preview: null, dragging: null, nextId: 2}}},
    workspaceViews: {w: {...view, tabs: {root: {id: 3, axis: 'x', ratio: 50, children: [{id: 1, tabs: ['dashboard'], active: 'dashboard'}, {...files, id: 2}]}, preview: null, dragging: null, nextId: 4}}},
  };
  localStorage.setItem('hypercube.views', JSON.stringify(saved));
  const store = fresh();
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  expect(store.get().errors).toEqual([]);
  expect(store.get().workbench.sizes).toEqual({sidebar: 290, rightPanel: 320, panel: 240});
  expect(panes(store.get().editor!.tabs.root).map(pane => pane.tabs)).toEqual([[mainTabId]]);
  store.update(state => selectSession(state, 'b'));
  expect(panes(store.get().editor!.tabs.root).map(pane => pane.tabs)).toEqual([[mainTabId], ['file:a.md']]);
  store.update(state => selectWorkspace(state, 'w'));
  expect(panes(store.get().editor!.tabs.root).map(pane => pane.tabs)).toEqual([[mainTabId], ['file:a.md']]);
});

test('workspace views persist independently and are removed with their workspace', () => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, [...workspaces, testWorkspace({id: 'other'})]));
  store.update(state => setExpanded(typed(state, 'a', 'unfinished'), 'activity', true));
  openRequestedFile(store, {session: 'a', path: 'session.md', lines: []});
  store.update(state => selectWorkspace(state, 'other'));
  openFileAt(store, 'workspace.md', [{start: 4}]);
  store.update(state => moveTab(state, 'file:workspace.md', 1, 'right'));
  store.update(state => rememberScroll(toggle(state, 'panel'), 'workspace:other', 'file:workspace.md', {top: 99, left: 0, following: false}));
  const tabs = store.get().editor!.tabs;
  store.update(state => openUnsent(state));
  expect(store.get().unsent?.workspace).toBe('other');
  store.update(state => typed(state, unsentKey('other'), 'new prompt'));
  store.update(state => selectSession(state, 'a'));
  expect(store.get().session?.expanded.activity).toBe(true);
  expect(store.get().editor?.files.open['session.md']).toBeDefined();
  expect(draftOf(store.get(), 'a').text).toBe('unfinished');
  store.update(state => selectWorkspace(state, 'other'));
  expect(store.get().editor?.tabs).toEqual(tabs);
  expect(draftOf(store.get(), unsentKey('other')).text).toBe('new prompt');
  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, [...workspaces, testWorkspace({id: 'other'})]));
  expect(reloaded.get().selected).toBe('workspace:other');
  expect(reloaded.get().session).toBeNull();
  expect(reloaded.get().editor?.tabs).toEqual(tabs);
  expect(reloaded.get().editor?.panel).toBe(true);
  expect(reloaded.get().editor?.scroll['file:workspace.md'].top).toBe(99);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().selected).toBeNull();
  expect(reloaded.get().editor).toBeNull();
  expect(reloaded.get().workspaceViews).toEqual({});
  expect(reloaded.get().fileTrees['workspace:other']).toBeUndefined();
});

test('hiding a collapsed region keeps its size and the other saved views', () => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  openFileAt(store, 'session.md', []);
  store.update(state => resizeRegion(state, 'rightPanel', 420));
  store.update(state => hideRegion(state, 'rightPanel'));

  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().errors).toEqual([]);
  expect(reloaded.get().selected).toBe('a');
  expect(reloaded.get().workbench.rightPanel).toBe(false);
  expect(reloaded.get().workbench.sizes.rightPanel).toBe(420);
  expect(panes(reloaded.get().editor!.tabs.root)[0].tabs).toEqual([mainTabId, 'file:session.md']);
});
