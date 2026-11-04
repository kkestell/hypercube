import {afterEach, expect, spyOn, test} from 'bun:test';
import {fileTabId, openBrowserTab, tabEntry} from '../editor/editor.js';
import {moveTab, openTab} from '../editor/tab-layout.js';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {createFilePoller} from './file-polling.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

// stubFetch answers each request from routes, by path and query, and records
// the requests in order. A route may return a promise to hold its answer.
function stubFetch(routes) {
  const requests = [];
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async url => {
    const {pathname, searchParams} = new URL(url, 'http://localhost/');
    const name = pathname.slice(1) + ':' + searchParams.get('path');
    requests.push(name);
    return (routes[name] ?? (() => Response.json([])))();
  });
  return requests;
}

const unread = {text: null, message: null, etag: null, deleted: false};

// A session with folders a (expanded), a/b (expanded), c/d (expanded under a
// collapsed c), and file tabs x.go (active in its pane) and y.go (behind z.go).
function attached(id = 's') {
  const session = emptySession(id);
  let tabs = [fileTabId('x.go'), fileTabId('y.go'), fileTabId('z.go')].reduce(openTab, session.tabs);
  tabs = moveTab(tabs, fileTabId('x.go'), tabs.root.id, 'right');
  const open = {'x.go': unread, 'y.go': unread, 'z.go': {...unread, text: 'z', etag: '"z"'}};
  return {...session, tabs, files: {listings: {}, expanded: {a: true, 'a/b': true, 'c/d': true}, open}};
}

const state = (extra = {}) => ({visible: true, session: attached(), workbench: {rightPanel: true, rightTab: 'files'}, ...extra});

test('nothing is polled while the page is hidden or unattached', async () => {
  const requests = stubFetch({});
  await createFilePoller(createStore(state({visible: false})))();
  await createFilePoller(createStore(state({session: null})))();
  expect(requests).toEqual([]);
});

test('only shown folders and the active file tab of each pane are polled', async () => {
  const requests = stubFetch({});
  await createFilePoller(createStore(state()))();
  expect(requests).toEqual(['files:', 'files:a', 'files:a/b', 'file:z.go', 'file:x.go']);
  expect(fetchMock.mock.calls[3][1].headers).toEqual({'If-None-Match': '"z"'});

  const hidden = stubFetch({});
  await createFilePoller(createStore(state({workbench: {rightPanel: true, rightTab: 'changes'}})))();
  expect(hidden).toEqual(['changes:null', 'file:z.go', 'file:x.go']);
});

test('a browser tab polls no file', async () => {
  const requests = stubFetch({});
  const session = {...emptySession('s'), files: {listings: {}, expanded: {}, open: {er: unread}}};
  const store = createStore(openBrowserTab({visible: true, session, workbench: {rightPanel: false}}, true));
  await createFilePoller(store)();
  expect(requests).toEqual([]);
});

test('requests go one at a time and a poll is skipped while one runs', async () => {
  let release;
  const requests = stubFetch({'files:': () => new Promise(resolve => { release = () => resolve(Response.json([])); })});
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
  stubFetch({'files:a/b': () => new Response('gone', {status: 404})});
  const store = createStore(state());
  await createFilePoller(store)();
  expect(store.get().session.files.expanded).toEqual({a: true, 'c/d': true});
});

test('results for a session the page left are dropped', async () => {
  const store = createStore(state());
  stubFetch({'files:': () => {
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
  const requests = stubFetch({
    'changes:null': () => Response.json({branch: 'main', files: []}),
    'diff:a.go': () => new Response('patch'),
  });
  const store = createStore(state({session, workbench: {rightPanel: true, rightTab: 'changes'}}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
  expect(store.get().session.diffs['a.go']).toEqual({text: 'patch', message: null});
  requests.length = 0;
  store.update(s => ({...s, workbench: {rightPanel: false, rightTab: 'changes'}}));
  await createFilePoller(store)();
  expect(requests).toEqual(['changes:null', 'diff:a.go']);
});


test('a diff title follows its status while the right panel shows Files', async () => {
  const session = emptySession('s');
  session.diffs = {'a.go': {text: '', message: null}};
  session.tabs = openTab(session.tabs, 'diff:a.go');
  session.changes = {branch: 'main', files: [{path: 'a.go', status: 'untracked'}]};
  const requests = stubFetch({
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
