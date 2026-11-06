import {afterEach, beforeEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {startDraftStorage} from '../agent/draft-storage.ts';
import {draftOf, typed, unsentKey} from '../agent/drafts.ts';
import {activatePane, keepOpen, moveTab, openDiffTab, openRequestedFile, rememberScroll, resizeSplit} from '../editor/editor.ts';
import {panes} from '../editor/tab-layout.ts';
import {receivedWorkspaces, selectSession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {receivedTranscript, setExpanded} from '../transcript/transcripts.ts';
import {startViewStorage} from './view-storage.ts';
import {toggleWrap} from './workbench.ts';

const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})]})];
const fresh = (selected = '') => createStore(testState({selected}));

beforeEach(() => localStorage.clear());
afterEach(() => { mock.restore(); localStorage.clear(); });

test('session switches and reloads preserve independent view choices without caching server data', async () => {
  stubFetch(() => new Response('file contents must not be saved'));
  const store = fresh('a');
  startViewStorage(store);
  store.update(state => receivedWorkspaces(state, workspaces));
  store.update(state => typed(state, 'a', 'unfinished prompt'));
  openRequestedFile(store, {session: 'a', path: 'guide.md', lines: [{start: 2}]});
  openDiffTab(store, 'changed.go');
  store.update(state => keepOpen(state, 'diff:changed.go'));
  store.update(state => moveTab(state, 'file:guide.md', 1, 'right'));
  const split = store.get().session!.tabs.root!.id;
  store.update(state => resizeSplit(state, split, 37));
  store.update(state => activatePane(state, panes(state.session!.tabs.root)[1].id));
  store.update(state => setExpanded(state, 'activity', true));
  store.update(state => ({...state, session: {...state.session!, files: {...state.session!.files, expanded: {src: true}}}}));
  store.update(state => rememberScroll(state, 'a', 'file:guide.md', {top: 120, left: 8, following: false}));
  store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: true, items: [{id: 'message', seq: 1, kind: 'text', text: 'transcript must not be saved'}]}));
  await Promise.resolve();
  const layout = store.get().session!.tabs;
  const activePane = store.get().session!.activePane;

  store.update(state => selectSession(state, 'b'));
  expect(store.get().sessionViews.a).toBeDefined();
  expect(store.get().sessionViews.b).toBeUndefined();
  expect(store.get().session!.tabs.root).toMatchObject({tabs: ['agent']});
  store.update(state => rememberScroll(state, 'a', 'a', {top: 345, left: 0, following: false}));
  store.update(state => selectSession(state, 'a'));
  expect(store.get().sessionViews.a).toBeUndefined();
  expect(store.get().session!.tabs).toEqual(layout);
  expect(store.get().session!.activePane).toBe(activePane);
  expect(store.get().session!.scroll.a.top).toBe(345);
  expect(draftOf(store.get(), 'a').text).toBe('unfinished prompt');

  const text = localStorage.getItem('hypercube.views')!;
  expect(text).not.toContain('contents must not');
  expect(text).not.toContain('transcript must not');
  const reloaded = fresh();
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  const restored = reloaded.get().session!;
  expect(reloaded.get().selected).toBe('a');
  expect(restored.tabs).toEqual(layout);
  expect(restored.activePane).toBe(activePane);
  expect(restored.files.expanded).toEqual({src: true});
  expect(restored.expanded.activity).toBe(true);
  expect(restored.scroll['file:guide.md']).toEqual({top: 120, left: 8, following: false});
  expect(restored.files.open['guide.md']).toMatchObject({text: null, etag: null, source: true, lines: [{start: 2}], reveal: false});
  expect(restored.diffs['changed.go']).toEqual({text: null, message: null});
  expect(restored.transcript.order).toEqual([]);
  expect(restored.transcriptReady).toBe(false);
});

test('an explicit session URL wins over saved selection and a restored new-session draft', () => {
  const previous = fresh('a');
  startViewStorage(previous);
  previous.update(state => receivedWorkspaces(state, workspaces));
  localStorage.setItem('hypercube.drafts', JSON.stringify({drafts: {[unsentKey('w')]: 'keep this draft'}, unsent: {workspace: 'w', profile: 'lead', worktree: true}}));
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
