import {afterEach, beforeEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {startDraftStorage} from '../agent/draft-storage.ts';
import {draftOf, typed, unsentKey} from '../agent/drafts.ts';
import {activatePane, keepOpen, moveTab, openDiffTab, openFileAt, openRequestedFile, rememberScroll, resizeSplit} from '../editor/editor.ts';
import {panes} from '../editor/tab-layout.ts';
import {openUnsent, receivedWorkspaces, selectSession, selectWorkspace} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {receivedTranscript, setExpanded} from '../transcript/transcripts.ts';
import {startViewStorage} from './view-storage.ts';
import {resizeRegion, toggle, toggleWrap} from './workbench.ts';

const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})]})];
const fresh = (selected = '') => createStore(testState({selected}));

beforeEach(() => localStorage.clear());
afterEach(() => { mock.restore(); localStorage.clear(); });

test('session switches and reloads preserve independent view choices without caching server data', async () => {
  stubFetch(() => new Response('file contents must not be saved'));
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  store.update(state => resizeRegion(state, 'editor', 640));
  store.update(state => typed(state, 'a', 'unfinished prompt'));
  openRequestedFile(store, {session: 'a', path: 'guide.md', lines: [{start: 2}]});
  openDiffTab(store, 'changed.go');
  store.update(state => keepOpen(state, 'diff:changed.go'));
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
  expect(store.get().editor!.tabs.root).toBeNull();
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
  expect(reloaded.get().workbench.sizes.editor).toBe(640);
  expect(restored.activePane).toBe(activePane);
  expect(restored.files.expanded).toEqual({src: true});
  expect(reloaded.get().session!.expanded.activity).toBe(true);
  expect(restored.scroll['file:guide.md']).toEqual({top: 120, left: 8, following: false});
  expect(restored.files.open['guide.md']).toMatchObject({text: null, etag: null, source: true, lines: [{start: 2}], reveal: false});
  expect(restored.diffs['changed.go']).toEqual({text: null, message: null});
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

test.each([false, true])('legacy terminal visibility migrates to the selected session only (open %p)', panel => {
  const previous = fresh('a');
  startViewStorage(previous);
  previous.update(state => receivedWorkspaces(state, workspaces));
  previous.update(state => selectSession(state, 'b'));
  previous.update(state => selectSession(state, 'a'));
  const saved = JSON.parse(localStorage.getItem('hypercube.views')!);
  delete saved.workspaceViews;
  saved.workbench.panel = panel;
  for (const view of Object.values(saved.sessionViews)) delete (view as {panel?: boolean}).panel;
  localStorage.setItem('hypercube.views', JSON.stringify(saved));

  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().errors).toEqual([]);
  expect(reloaded.get().editor?.panel).toBe(panel);
  expect(reloaded.get().sessionViews.b.panel).toBe(false);
  expect(JSON.parse(localStorage.getItem('hypercube.views')!).workbench.panel).toBeUndefined();
});

test('an explicit session URL wins over saved selection and a restored new-session draft', () => {
  const previous = fresh('a');
  startViewStorage(previous);
  previous.update(state => receivedWorkspaces(state, workspaces));
  localStorage.setItem('hypercube.drafts', JSON.stringify({drafts: {[unsentKey('w')]: 'keep this draft'}, unsent: {workspace: 'w', settings: null, worktree: true}}));
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

test('legacy wrap settings migrate and unavailable storage keeps in-memory changes usable', () => {
  localStorage.setItem('hypercube.wrap', 'true');
  localStorage.setItem('hypercube.fixedWidth', 'false');
  const store = fresh();
  startViewStorage(store);
  expect(store.get().workbench).toMatchObject({wrap: true, fixedWidth: false});
  expect(localStorage.getItem('hypercube.wrap')).toBeNull();
  expect(localStorage.getItem('hypercube.fixedWidth')).toBeNull();
  const storage = localStorage;
  Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: {setItem() { throw new Error('quota exceeded'); }}});
  try {
    store.update(toggleWrap);
    store.update(toggleWrap);
    expect(store.get().workbench.wrap).toBe(true);
    expect(store.get().errors).toHaveLength(1);
    expect(store.get().errors[0].message).toContain('View state could not be saved');
  } finally {
    Object.defineProperty(globalThis, 'localStorage', {configurable: true, value: storage});
  }
  store.update(toggleWrap);
  expect(JSON.parse(localStorage.getItem('hypercube.views')!).workbench.wrap).toBe(false);
});

test('invalid saved layouts report their failure and retain defaults', () => {
  localStorage.setItem('hypercube.views', '{"selected":"a","workbench":{},"sessionViews":{}}');
  const store = fresh();
  startViewStorage(store);
  expect(store.get().sessionViews).toEqual({});
  expect(store.get().workbench.sizes.sidebar).toBe(290);
  expect(store.get().errors[0].message).toContain('Could not restore view state');
});

test('saved agent tabs migrate away without losing files, splits, or transcript scroll', () => {
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  openFileAt(store, 'guide.md', [{start: 2}]);
  openFileAt(store, 'other.py', []);
  store.update(state => moveTab(state, 'file:other.py', 1, 'right'));
  const saved = JSON.parse(localStorage.getItem('hypercube.views')!);
  const filePane = saved.sessionViews.a.tabs.root;
  saved.sessionViews.a.tabs = {...saved.sessionViews.a.tabs, nextId: 12, root: {
    id: 11, axis: 'x', ratio: 37, children: [{id: 10, tabs: ['agent'], active: 'agent'}, filePane],
  }};
  saved.sessionViews.a.activePane = 10;
  saved.sessionViews.a.scroll.a = {top: 350, left: 0, following: false};
  saved.sessionViews.b = {...saved.sessionViews.a, files: {}, tabs: {
    root: {id: 1, tabs: ['agent'], active: 'agent'}, preview: null, dragging: null, nextId: 2,
  }};
  delete saved.workbench.sizes.editor;
  localStorage.setItem('hypercube.views', JSON.stringify(saved));

  const restored = fresh();
  startViewStorage(restored);
  restored.update(state => receivedWorkspaces(state, workspaces));
  expect(restored.get().errors).toEqual([]);
  expect(restored.get().editor?.tabs.root).toEqual(filePane);
  expect(restored.get().editor?.activePane).toBeNull();
  expect(restored.get().editor?.files.open['guide.md']).toMatchObject({source: true, lines: [{start: 2}]});
  expect(restored.get().editor?.scroll.a).toEqual({top: 350, left: 0, following: false});
  expect(restored.get().workbench.sizes.editor).toBe(560);
  restored.update(state => selectSession(state, 'b'));
  expect(restored.get().editor?.tabs.root).toBeNull();
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
