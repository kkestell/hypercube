import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptyEditor} from '../editor/attachment.ts';
import {closeTab, keepOpen, openDiffTab, rememberScroll, tabEntry, tabPath} from '../editor/editor.ts';
import {moveTab, panes} from '../editor/tab-layout.ts';
import {editorView, restoreView} from '../editor/view-state.ts';
import {createStore} from '../store.ts';
import {loadChanges, readOpenDiff} from './changes.ts';

afterEach(() => mock.restore());

test('comparison loads are independent, newest-only, and cannot cross attachments', async () => {
  const replies: Array<(response: Response) => void> = [];
  stubFetch(() => new Promise<Response>(resolve => replies.push(resolve)));
  const store = createStore(testState({editor: emptyEditor({session: 's'})}));
  const uncommitted = loadChanges(store);
  const oldBranch = loadChanges(store, 'branch');
  const branch = loadChanges(store, 'branch');
  replies[2](Response.json({branch: 'feature', target: 'main', files: [{path: 'b', status: 'added'}, {path: 'a', status: 'added'}]}));
  await branch;
  replies[1](new Response('old failure', {status: 500}));
  await oldBranch;
  replies[0](new Response('uncommitted failed', {status: 500}));
  await uncommitted;
  expect(store.get().editor?.changes).toEqual({error: 'uncommitted failed'});
  expect(store.get().editor?.branchChanges).toMatchObject({target: 'main', files: [{path: 'a'}, {path: 'b'}]});
  const departing = loadChanges(store, 'branch');
  store.update(state => ({...state, editorVersion: state.editorVersion + 1, editor: emptyEditor({session: 's'})}));
  replies[3](Response.json({branch: 'old', target: 'old', files: []}));
  await departing;
  expect(store.get().editor?.branchChanges).toBeNull();
});

test('same literal path has separate patches, titles, scroll, restoration, and close ownership', async () => {
  const requests: URL[] = [];
  stubFetch(url => {
    const request = new URL(url, 'http://x/');
    requests.push(request);
    return new Response(request.searchParams.get('comparison') === 'branch' ? 'branch patch' : 'uncommitted patch');
  });
  const path = 'src/a:diff:b.go';
  const workingId = 'diff:' + path, branchId = 'branch-diff:' + path;
  const store = createStore(testState({editor: {...emptyEditor({session: 's'}),
    changes: {branch: 'feature', files: [{path, status: 'modified'}]},
    branchChanges: {branch: 'feature', target: 'main', files: [{path, status: 'added'}]},
  }}));
  openDiffTab(store, path);
  store.update(state => keepOpen(state, workingId));
  openDiffTab(store, path, 'branch');
  store.update(state => ({...state, editor: {...state.editor!, tabs: moveTab(state.editor!.tabs, branchId, 1, 'right')}}));
  await Promise.all([readOpenDiff(store, path), readOpenDiff(store, path, 'branch')]);
  expect(panes(store.get().editor!.tabs.root)).toHaveLength(2);
  expect(requests.map(request => [request.searchParams.get('path'), request.searchParams.get('comparison')])).toEqual([[path, null], [path, 'branch']]);
  expect(store.get().editor?.diffs[workingId]?.text).toBe('uncommitted patch');
  expect(store.get().editor?.diffs[branchId]?.text).toBe('branch patch');
  expect(tabPath(branchId)).toBe(path);
  expect(tabEntry(store.get(), workingId).label).toBe('a:diff:b.go (Uncommitted)');
  expect(tabEntry(store.get(), branchId).label).toBe('a:diff:b.go (Branch changes) (Added)');
  expect(tabEntry(store.get(), branchId).title).toBe(`${path} (Branch changes)`);
  for (const [id, top] of [[workingId, 10], [branchId, 50]] as const) store.update(state => rememberScroll(state, 's', id, {top, left: 0, following: false}));
  const restored = restoreView(emptyEditor({session: 's'}), editorView(store.get().editor!));
  expect(restored.diffs).toEqual({[workingId]: {text: null, message: null}, [branchId]: {text: null, message: null}});
  expect(restored.scroll[branchId]?.top).toBe(50);
  store.update(state => closeTab(state, workingId));
  expect(store.get().editor?.diffs[workingId]).toBeUndefined();
  expect(store.get().editor?.diffs[branchId]?.text).toBe('branch patch');
  expect(store.get().editor?.scroll[branchId]?.top).toBe(50);
  // Replacing the branch preview forgets only that comparison.
  openDiffTab(store, 'other', 'branch');
  expect(store.get().editor?.diffs[branchId]).toBeUndefined();
});

test('failed and superseded branch diff reads retain the latest patch independently', async () => {
  const replies: Array<(response: Response) => void> = [];
  stubFetch(() => new Promise<Response>(resolve => replies.push(resolve)));
  const store = createStore(testState({editor: {...emptyEditor({session: 's'}), diffs: {
    'diff:a': {text: 'working', message: null}, 'branch-diff:a': {text: 'previous', message: null},
  }}}));
  const old = readOpenDiff(store, 'a', 'branch');
  const newest = readOpenDiff(store, 'a', 'branch');
  replies[1](new Response('new branch'));
  await newest;
  replies[0](new Response('old branch'));
  await old;
  const failed = readOpenDiff(store, 'a', 'branch');
  replies[2](new Response('failed', {status: 500}));
  await failed;
  expect(store.get().editor?.diffs['branch-diff:a']?.text).toBe('new branch');
  expect(store.get().editor?.diffs['diff:a']?.text).toBe('working');
});
