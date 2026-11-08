import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {fileTabId, tabEntry} from '../editor/editor.ts';
import {moveTab, openTab, panes} from '../editor/tab-layout.ts';
import {emptyEditor, emptySession} from '../sessions/selection.ts';
import type {RightTab, Workbench} from '../app/types.ts';
import type {EditorView} from '../editor/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {testSession, testState, testWorkspace, unread} from '../../test-state.ts';
import {loadChanges, readOpenDiff} from '../changes/changes.ts';
import {receivedWorkspaces, selectSession, selectWorkspace} from '../sessions/selection.ts';
import {readOpenFile} from './files.ts';
import {createFilePoller} from './file-polling.ts';

afterEach(() => mock.restore());

const listings = {'': [{name: 'a', dir: true}, {name: 'c', dir: true}], a: [{name: 'b', dir: true}], c: [{name: 'd', dir: true}]};

// stubRoutes answers each request from routes, by path and query, and records
// the requests in order. A route may return a promise to hold its answer.
function stubRoutes(routes: Record<string, () => Response | Promise<Response>>) {
  const requests: string[] = [];
  const fetches = stubFetch(url => {
    const {pathname, searchParams} = new URL(url, 'http://localhost/');
    const name = pathname.slice(1) + ':' + searchParams.get('path');
    requests.push(name);
    if (routes[name]) return routes[name]();
    return Response.json(pathname === '/files/tree' ? listings : []);
  });
  return {requests, fetches};
}

// A session with folders a (expanded), a/b (expanded), c/d (expanded under a
// collapsed c), and file tabs x.go (active in its pane) and y.go (behind z.go).
function attached(id = 's'): EditorView {
  const session = emptyEditor({session: id});
  let tabs = [fileTabId('x.go'), fileTabId('y.go'), fileTabId('z.go')].reduce(openTab, session.tabs);
  tabs = moveTab(tabs, fileTabId('x.go'), panes(tabs.root)[0].id, 'right');
  const open = {'x.go': unread, 'y.go': unread, 'z.go': {...unread, text: 'z', etag: '"z"'}};
  return {...session, tabs, files: {listings, listingErrors: {}, expanded: {a: true, 'a/b': true, 'c/d': true}, open}};
}

const workbench = (rightPanel: boolean, rightTab: RightTab): Workbench => ({...testState().workbench, rightPanel, rightTab});

const state = (overrides: Partial<State> = {}) => testState({editor: attached(), workbench: workbench(true, 'files'), ...overrides});

test('nothing is polled while the page is hidden or unattached', async () => {
  const {requests} = stubRoutes({});
  await createFilePoller(createStore(state({visible: false})))();
  await createFilePoller(createStore(state({editor: null})))();
  expect(requests).toEqual([]);
});

test('chat branch refreshes even with the right panel closed and no file tabs', async () => {
  const store = createStore(receivedWorkspaces(testState({selected: 's', workbench: workbench(false, 'search')}), [testWorkspace({id: 'w', sessions: [testSession({id: 's'})]})]));
  const {requests} = stubRoutes({'changes:null': () => Response.json({branch: 'feature', files: []})});
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null']);
  expect(store.get().editor?.changes).toEqual({branch: 'feature', files: []});
});

test('polls each needed comparison once and matches both active same-path diffs', async () => {
  const session = emptyEditor({session: 's'});
  session.gitComparison = 'branch';
  session.diffs = {'diff:a:b': {text: null, message: null}, 'branch-diff:a:b': {text: null, message: null}};
  session.tabs = openTab(openTab(session.tabs, 'diff:a:b'), 'branch-diff:a:b');
  session.tabs = moveTab(session.tabs, 'branch-diff:a:b', 1, 'right');
  const requests: string[] = [];
  stubFetch(url => {
    const {pathname, searchParams} = new URL(url, 'http://x/');
    requests.push(`${pathname}:${searchParams.get('comparison') ?? 'uncommitted'}:${searchParams.get('path') ?? ''}`);
    return pathname === '/changes' ? Response.json({branch: 'feature', files: []}) : new Response('patch');
  });
  const store = createStore(state({editor: session, session: emptySession('s'), workbench: workbench(true, 'git')}));
  await createFilePoller(store)();
  expect(requests).toEqual(['/changes:uncommitted:', '/changes:branch:', '/diff:uncommitted:a:b', '/diff:branch:a:b']);
  requests.length = 0;
  // An inactive branch diff does not request branch data, even in a chat.
  store.update(state => ({...state, workbench: workbench(false, 'git'), editor: {...state.editor!, tabs: {...state.editor!.tabs, root: {id: 1, tabs: ['diff:a:b', 'branch-diff:a:b'], active: 'diff:a:b'}}}}));
  await createFilePoller(store)();
  expect(requests).toEqual(['/changes:uncommitted:', '/diff:uncommitted:a:b']);
});

test('only shown folders, their changes, and the active file tab of each pane are polled', async () => {
  const {requests, fetches} = stubRoutes({});
  await createFilePoller(createStore(state()))();
  expect(requests).toEqual(['files/tree:null', 'changes:null', 'file:z.go', 'file:x.go']);
  expect(JSON.parse(fetches.mock.calls[0]?.[1]?.body as string)).toEqual(['', 'a', 'a/b']);
  expect(fetches.mock.calls[2]?.[1]?.headers).toEqual({'If-None-Match': '"z"'});

  mock.restore();
  const hidden = stubRoutes({}).requests;
  await createFilePoller(createStore(state({workbench: workbench(true, 'search')})))();
  expect(hidden).toEqual(['file:z.go', 'file:x.go']);
});

test('one poll restores nested expanded folders using freshly loaded parent listings', async () => {
  const session = {...attached(), files: {...attached().files, listings: {}}};
  const {requests} = stubRoutes({});
  const store = createStore(state({editor: session}));
  await createFilePoller(store)();
  expect(requests[0]).toBe('files/tree:null');
  expect(store.get().editor?.files.listings.a).toEqual([{name: 'b', dir: true}]);
});

test('Git polls changes without folder listings only when visible', async () => {
  const {requests} = stubRoutes({});
  const store = createStore(state({workbench: workbench(true, 'git')}));
  const poll = createFilePoller(store);
  await poll();
  expect(requests).toEqual(['changes:null', 'file:z.go', 'file:x.go']);
  requests.length = 0;
  store.update(s => ({...s, workbench: workbench(false, 'git')}));
  await poll();
  expect(requests).toEqual(['file:z.go', 'file:x.go']);
  requests.length = 0;
  store.update(s => ({...s, visible: false, workbench: workbench(true, 'git')}));
  await poll();
  expect(requests).toEqual([]);
});

test('a folder shown only through its deleted files stays expanded', async () => {
  const {requests} = stubRoutes({'files/tree:null': () => Response.json({'': [], gone: null})});
  const session = {...emptyEditor({session: 's'}), files: {listings: {'': []}, listingErrors: {}, expanded: {gone: true as const}, open: {}}};
  const store = createStore(state({editor: session}));
  await createFilePoller(store)();
  expect(requests).toEqual(['files/tree:null', 'changes:null']);
  expect(store.get().editor?.files.expanded.gone).toBe(true);
});



test('requests go one at a time and a poll is skipped while one runs', async () => {
  let release = () => {};
  const {requests} = stubRoutes({'files/tree:null': () => new Promise(resolve => { release = () => resolve(Response.json(listings)); })});
  const poll = createFilePoller(createStore(state()));
  const first = poll();
  await poll();
  await Promise.resolve();
  expect(requests).toEqual(['files/tree:null']);
  release();
  await first;
  expect(requests.length).toBe(4);
});

test('a missing folder collapses', async () => {
  stubRoutes({'files/tree:null': () => Response.json({...listings, 'a/b': null})});
  const store = createStore(state());
  await createFilePoller(store)();
  expect(store.get().editor?.files.expanded).toEqual({a: true, 'c/d': true});
});

test('a missing session folder replaces a cached tree with the missing message', async () => {
  stubRoutes({'files/tree:null': () => new Response('gone', {status: 404})});
  const store = createStore(state());
  await createFilePoller(store)();
  expect(store.get().editor?.files.listings['']).toBeNull();
});

test('results for a session the page left are dropped', async () => {
  const store = createStore(state());
  stubRoutes({'files/tree:null': () => {
    store.update(s => ({...s, editor: emptyEditor({session: 'other'})}));
    return Response.json(listings);
  }});
  await createFilePoller(store)();
  expect(store.get().editor).toEqual(emptyEditor({session: 'other'}));
});


test('Changes refreshes with an active diff even while hidden', async () => {
  const session = emptyEditor({session: 's'});
  session.diffs = {'diff:a.go': {text: null, message: null}};
  session.tabs = openTab(session.tabs, 'diff:a.go');
  const {requests} = stubRoutes({
    'changes:null': () => Response.json({branch: 'main', files: []}),
    'diff:a.go': () => new Response('patch'),
  });
  const store = createStore(state({editor: session, workbench: workbench(true, 'search')}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
  expect(store.get().editor?.diffs['diff:a.go']).toEqual({text: 'patch', message: null});
  requests.length = 0;
  store.update(s => ({...s, workbench: workbench(false, 'search')}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
});


test('a diff title follows its status while the right panel shows Files', async () => {
  const session = emptyEditor({session: 's'});
  session.diffs = {'diff:a.go': {text: '', message: null}};
  session.tabs = openTab(session.tabs, 'diff:a.go');
  session.changes = {branch: 'main', files: [{path: 'a.go', status: 'untracked'}]};
  const {requests} = stubRoutes({
    'files/tree:null': () => Response.json({'': []}),
    'changes:null': () => Response.json({branch: 'main', files: [{path: 'a.go', status: 'added'}]}),
    'diff:a.go': () => new Response('patch'),
  });
  const store = createStore(state({editor: session}));
  expect(tabEntry(store.get(), 'diff:a.go').label).toBe('a.go (Uncommitted) (Untracked)');
  await createFilePoller(store)();
  expect(requests).toEqual(['files/tree:null', 'changes:null', 'diff:a.go']);
  expect(tabEntry(store.get(), 'diff:a.go').label).toBe('a.go (Uncommitted) (Added)');
});

for (const kind of ['file', 'diff'] as const) for (const dashboard of [false, true]) {
  test(`a newer polled ${kind} supersedes overlapping reads and an earlier A-B-A attachment (workspace ${dashboard})`, async () => {
    const store = createStore(receivedWorkspaces(testState({selected: 'a', workbench: workbench(false, 'search')}), [testWorkspace({
      id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})],
    })]));
    const selectA = (state: State) => dashboard ? selectWorkspace(state, 'w') : selectSession(state, 'a');
    if (dashboard) store.update(selectA);
    store.update(state => {
      const session = state.editor!;
      return {...state, editor: {...session, tabs: openTab(session.tabs, `${kind}:a.go`),
        files: {...session.files, open: {'a.go': unread}}, diffs: {'diff:a.go': {text: null, message: null}}}};
    });
    const replies: Array<(response: Response) => void> = [];
    let started = () => {};
    const polled = new Promise<void>(resolve => { started = resolve; });
    stubFetch(url => {
      if (new URL(url, 'http://x/').pathname !== `/${kind}`) return Response.json({branch: 'main', files: []});
      return new Promise<Response>(resolve => {
        replies.push(resolve);
        if (replies.length === 4) started();
      });
    });
    const read = kind === 'file' ? readOpenFile : readOpenDiff;
    const firstRead = read(store, 'a.go');
    const oldAttachment = read(store, 'a.go');
    store.update(state => selectSession(state, 'b'));
    store.update(selectA);
    const older = read(store, 'a.go');
    const newer = createFilePoller(store)();
    await polled;
    replies[3](new Response('new text'));
    await newer;
    replies[2](new Response('old text'));
    await older;
    replies[0](new Response('first read'));
    await firstRead;
    // This read and the poll both have version 2 in their own attachments.
    replies[1](new Response('old attachment'));
    await oldAttachment;
    const session = store.get().editor!;
    expect((kind === 'file' ? session.files.open['a.go'] : session.diffs['diff:a.go']).text).toBe('new text');
  });
}

test('a newer change poll supersedes an overlapping initial read', async () => {
  const store = createStore(state({editor: emptyEditor({session: 's'})}));
  const replies: Array<(response: Response) => void> = [];
  let started = () => {};
  const polled = new Promise<void>(resolve => { started = resolve; });
  stubFetch(url => {
    if (new URL(url, 'http://x/').pathname !== '/changes') return Response.json({'': []});
    return new Promise<Response>(resolve => {
      replies.push(resolve);
      if (replies.length === 2) started();
    });
  });
  const initial = loadChanges(store);
  const poll = createFilePoller(store)();
  await polled;
  const latest = {branch: 'main', files: [{path: 'a.go', status: 'modified' as const}]};
  replies[1](Response.json(latest));
  await poll;
  replies[0](Response.json({branch: 'main', files: []}));
  await initial;
  expect(store.get().editor?.changes).toEqual(latest);
});
