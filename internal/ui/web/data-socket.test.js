import {afterEach, beforeEach, expect, jest, spyOn, test} from 'bun:test';
import {startDataSocket} from './data-socket.js';
import {selectSession} from './sessions/selection.js';
import {findPane} from './editor/tab-layout.js';
import {createStore} from './store.js';

class FakeSocket {
  static OPEN = 1;
  static all = [];
  constructor(url) {
    this.url = String(url);
    this.sent = [];
    this.readyState = 0;
    FakeSocket.all.push(this);
  }
  send(data) { this.sent.push(JSON.parse(data)); }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen();
  }
  receive(message) { this.onmessage({data: JSON.stringify(message)}); }
}

const realSocket = globalThis.WebSocket;
let reload;
beforeEach(() => {
  FakeSocket.all = [];
  globalThis.WebSocket = FakeSocket;
  reload = spyOn(location, 'reload').mockImplementation(() => {});
  jest.useFakeTimers();
});
afterEach(() => {
  globalThis.WebSocket = realSocket;
  reload.mockRestore();
  jest.useRealTimers();
});

const initial = () => ({workspaces: [], requests: [], selected: '', pending: null, unsent: null, drafts: {}, dismissed: {}, session: null});
const state = (build, running = true) => ({type: 'state', build, workspaces: [{id: 'w', name: 'repo', sessions: [{id: 'a', title: 'A', running, children: []}]}], requests: []});

test('state and transcript messages reach the store, and select follows the attached session', () => {
  const store = createStore(initial());
  startDataSocket(store);
  const [socket] = FakeSocket.all;
  expect(socket.url).toBe('ws://localhost/state');
  socket.open();
  expect(socket.sent).toEqual([{select: ''}]);
  socket.receive(state('b1'));
  expect(store.get().session.id).toBe('a');
  expect(socket.sent).toEqual([{select: ''}, {select: 'a'}]);
  socket.receive({type: 'transcript', session: 'a', reset: true, items: [{id: '1', seq: 1, kind: 'text', text: 'hi'}]});
  expect(store.get().session.transcript.order).toEqual(['1']);
  socket.receive(state('b1', false));
  expect(socket.sent.at(-1)).toEqual({select: ''});
});

test('it reconnects after a close and selects again', () => {
  const store = createStore(initial());
  startDataSocket(store);
  FakeSocket.all[0].open();
  FakeSocket.all[0].receive(state('b1'));
  FakeSocket.all[0].onclose();
  expect(FakeSocket.all.length).toBe(1);
  jest.advanceTimersByTime(1000);
  expect(FakeSocket.all.length).toBe(2);
  FakeSocket.all[1].open();
  expect(FakeSocket.all[1].sent).toEqual([{select: 'a'}]);
});

test('a reset transcript after a reconnect replaces the old items', () => {
  const store = createStore(initial());
  startDataSocket(store);
  const [first] = FakeSocket.all;
  first.open();
  first.receive(state('b1'));
  const item = (id, seq, text) => ({id, seq, kind: 'text', text});
  first.receive({type: 'transcript', session: 'a', reset: true, items: [item('1', 1, 'old'), item('2', 2, 'older')]});
  first.onclose();
  jest.advanceTimersByTime(1000);
  const second = FakeSocket.all[1];
  second.open();
  second.receive(state('b1'));
  second.receive({type: 'transcript', session: 'a', reset: true, items: [item('3', 1, 'new')]});
  const {order, items} = store.get().session.transcript;
  expect(order).toEqual(['3']);
  expect(items['3'].text).toBe('new');
});

test('a changed build reloads the page and the first one does not', () => {
  const store = createStore(initial());
  startDataSocket(store);
  FakeSocket.all[0].open();
  FakeSocket.all[0].receive(state('b1'));
  FakeSocket.all[0].onclose();
  jest.advanceTimersByTime(1000);
  FakeSocket.all[1].open();
  FakeSocket.all[1].receive(state('b1'));
  expect(reload).not.toHaveBeenCalled();
  FakeSocket.all[1].receive(state('b2'));
  expect(reload).toHaveBeenCalledTimes(1);
});

const fileText = (store, path) => new Promise(resolve => {
  const unsubscribe = store.subscribe(state => {
    const text = state.session.files.open[path]?.text;
    if (text === null || text === undefined) return;
    unsubscribe();
    resolve(text);
  });
});

test('an open message keeps and selects a file and reads its contents', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('hello'));
  try {
    const store = createStore(initial());
    startDataSocket(store);
    const [socket] = FakeSocket.all;
    socket.open();
    socket.receive(state('b1'));
    socket.receive({type: 'open', session: 'a', path: 'a.go', lines: []});
    expect(store.get().session.tabs.preview).toBeNull();
    expect(findPane(store.get().session.tabs.root, 'file:a.go').active).toBe('file:a.go');
    expect(await fileText(store, 'a.go')).toBe('hello');
    expect(fetches).toHaveBeenCalledTimes(1);
  } finally { fetches.mockRestore(); }
});

test('an open received during a session switch waits until the page returns', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('hello'));
  try {
    const store = createStore(initial());
    startDataSocket(store);
    const [socket] = FakeSocket.all;
    socket.open();
    const message = state('b1');
    message.workspaces[0].sessions.push({id: 'b', title: 'B', running: true, children: []});
    socket.receive(message);
    store.update(state => selectSession(state, 'b'));
    socket.receive({type: 'open', session: 'a', path: 'guide.md', lines: [{start: 1, end: 1}]});
    expect(store.get().session.files.open).toEqual({});
    expect(fetches).not.toHaveBeenCalled();
    store.update(state => selectSession(state, 'a'));
    expect(findPane(store.get().session.tabs.root, 'file:guide.md').active).toBe('file:guide.md');
    expect(store.get().session.files.open['guide.md']).toMatchObject({source: true, lines: [{start: 1, end: 1}]});
    expect(store.get().fileOpens).toEqual({});
    expect(await fileText(store, 'guide.md')).toBe('hello');
    expect(fetches).toHaveBeenCalledTimes(1);
    store.update(state => selectSession(state, 'b'));
    store.update(state => selectSession(state, 'a'));
    expect(store.get().session.files.open).toEqual({});
  } finally { fetches.mockRestore(); }
});
