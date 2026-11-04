import {afterEach, expect, mock, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {fileTabId, openBrowserTab, tabEntry} from '../editor/editor.ts';
import {moveTab, openTab, panes} from '../editor/tab-layout.ts';
import {emptySession} from '../sessions/selection.ts';
import type {RightTab, Workbench} from '../app/types.ts';
import type {AttachedSession} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {testState, unread} from '../../test-state.ts';
import {createFilePoller} from './file-polling.ts';

afterEach(() => mock.restore());

// stubRoutes answers each request from routes, by path and query, and records
// the requests in order. A route may return a promise to hold its answer.
function stubRoutes(routes: Record<string, () => Response | Promise<Response>>) {
  const requests: string[] = [];
  const fetches = stubFetch(url => {
    const {pathname, searchParams} = new URL(url, 'http://localhost/');
    const name = pathname.slice(1) + ':' + searchParams.get('path');
    requests.push(name);
    return (routes[name] ?? (() => Response.json([])))();
  });
  return {requests, fetches};
}

// A session with folders a (expanded), a/b (expanded), c/d (expanded under a
// collapsed c), and file tabs x.go (active in its pane) and y.go (behind z.go).
function attached(id = 's'): AttachedSession {
  const session = emptySession(id);
  let tabs = [fileTabId('x.go'), fileTabId('y.go'), fileTabId('z.go')].reduce(openTab, session.tabs);
  tabs = moveTab(tabs, fileTabId('x.go'), panes(tabs.root)[0].id, 'right');
  const open = {'x.go': unread, 'y.go': unread, 'z.go': {...unread, text: 'z', etag: '"z"'}};
  return {...session, tabs, files: {listings: {}, expanded: {a: true, 'a/b': true, 'c/d': true}, open}};
}

const workbench = (rightPanel: boolean, rightTab: RightTab): Workbench => ({...testState().workbench, rightPanel, rightTab});

const state = (overrides: Partial<State> = {}) => testState({session: attached(), workbench: workbench(true, 'files'), ...overrides});

test('nothing is polled while the page is hidden or unattached', async () => {
  const {requests} = stubRoutes({});
  await createFilePoller(createStore(state({visible: false})))();
  await createFilePoller(createStore(state({session: null})))();
  expect(requests).toEqual([]);
});

test('only shown folders and the active file tab of each pane are polled', async () => {
  const {requests, fetches} = stubRoutes({});
  await createFilePoller(createStore(state()))();
  expect(requests).toEqual(['files:', 'files:a', 'files:a/b', 'file:z.go', 'file:x.go']);
  expect(fetches.mock.calls[3]?.[1]?.headers).toEqual({'If-None-Match': '"z"'});

  mock.restore();
  const hidden = stubRoutes({}).requests;
  await createFilePoller(createStore(state({workbench: workbench(true, 'changes')})))();
  expect(hidden).toEqual(['changes:null', 'file:z.go', 'file:x.go']);
});

test('a browser tab polls no file', async () => {
  const {requests} = stubRoutes({});
  const session = {...emptySession('s'), files: {listings: {}, expanded: {}, open: {er: unread}}};
  const store = createStore(openBrowserTab(testState({session, workbench: workbench(false, 'files')}), true));
  await createFilePoller(store)();
  expect(requests).toEqual([]);
});

test('requests go one at a time and a poll is skipped while one runs', async () => {
  let release = () => {};
  const {requests} = stubRoutes({'files:': () => new Promise(resolve => { release = () => resolve(Response.json([])); })});
  const poll = createFilePoller(createStore(state()));
  const first = poll();
  await poll();
  await Promise.resolve();
  expect(requests).toEqual(['files:']);
  release();
  await first;
  expect(requests.length).toBe(5);
});

test('a missing folder collapses', async () => {
  stubRoutes({'files:a/b': () => new Response('gone', {status: 404})});
  const store = createStore(state());
  await createFilePoller(store)();
  expect(store.get().session?.files.expanded).toEqual({a: true, 'c/d': true});
});

test('results for a session the page left are dropped', async () => {
  const store = createStore(state());
  stubRoutes({'files:': () => {
    store.update(s => ({...s, session: emptySession('other')}));
    return Response.json([{name: 'a', dir: true}]);
  }});
  await createFilePoller(store)();
  expect(store.get().session).toEqual(emptySession('other'));
});


test('Changes refreshes with an active diff even while hidden', async () => {
  const session = emptySession('s');
  session.diffs = {'a.go': {text: null, message: null}};
  session.tabs = openTab(session.tabs, 'diff:a.go');
  const {requests} = stubRoutes({
    'changes:null': () => Response.json({branch: 'main', files: []}),
    'diff:a.go': () => new Response('patch'),
  });
  const store = createStore(state({session, workbench: workbench(true, 'changes')}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
  expect(store.get().session?.diffs['a.go']).toEqual({text: 'patch', message: null});
  requests.length = 0;
  store.update(s => ({...s, workbench: workbench(false, 'changes')}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
});


test('a diff title follows its status while the right panel shows Files', async () => {
  const session = emptySession('s');
  session.diffs = {'a.go': {text: '', message: null}};
  session.tabs = openTab(session.tabs, 'diff:a.go');
  session.changes = {branch: 'main', files: [{path: 'a.go', status: 'untracked'}]};
  const {requests} = stubRoutes({
    'files:': () => Response.json([]),
    'changes:null': () => Response.json({branch: 'main', files: [{path: 'a.go', status: 'added'}]}),
    'diff:a.go': () => new Response('patch'),
  });
  const store = createStore(state({session}));
  expect(tabEntry(store.get(), 'diff:a.go').label).toBe('a.go (Untracked)');
  await createFilePoller(store)();
  expect(requests).toEqual(['files:', 'changes:null', 'diff:a.go']);
  expect(tabEntry(store.get(), 'diff:a.go').label).toBe('a.go (Added)');
});
