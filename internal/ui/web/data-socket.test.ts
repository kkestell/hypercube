import {afterEach, beforeEach, expect, jest, mock, spyOn, test} from 'bun:test';
import {testSession, testState, testWorkspace} from '../test-state.ts';
import {startDataSocket, type ServerMessage, type StateMessage} from './data-socket.ts';
import {findPane} from './editor/tab-layout.ts';
import {selectSession} from './sessions/selection.ts';
import {createStore, type Store} from './store.ts';
import type {TranscriptItem} from './transcript/types.ts';

class FakeSocket {
  static OPEN = 1;
  static all: FakeSocket[] = [];
  url: string;
  sent: unknown[] = [];
  readyState = 0;
  onopen = () => {};
  onmessage = (_event: {data: string}) => {};
  onclose = () => {};
  constructor(url: URL) {
    this.url = String(url);
    FakeSocket.all.push(this);
  }
  send(data: string) { this.sent.push(JSON.parse(data)); }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen();
  }
  receive(message: ServerMessage) { this.onmessage({data: JSON.stringify(message)}); }
}

const realSocket = globalThis.WebSocket;
const useSocket = (value: unknown) => Object.defineProperty(globalThis, 'WebSocket', {value, configurable: true, writable: true});
beforeEach(() => {
  FakeSocket.all = [];
  useSocket(FakeSocket);
  jest.useFakeTimers();
});
afterEach(() => {
  useSocket(realSocket);
  mock.restore();
  jest.useRealTimers();
});

const started = () => {
  const store = createStore(testState());
  startDataSocket(store);
  return store;
};

const stateMessage = (build: string, ended = false): StateMessage => ({
  type: 'state', build, requests: [], limits: [],
  workspaces: [testWorkspace({id: 'w', name: 'repo', sessions: [testSession({id: 'a', title: 'A', ended})]})],
});

const textItem = (id: string, seq: number, text: string): TranscriptItem => ({id, seq, kind: 'text', text});

test('state and transcript messages reach the store, and select follows the attached session', () => {
  const store = started();
  const [socket] = FakeSocket.all;
  expect(socket.url).toBe('ws://localhost/state');
  socket.open();
  expect(socket.sent).toEqual([{select: ''}]);
  socket.receive(stateMessage('b1'));
  expect(store.get().loaded).toBe(true);
  expect(store.get().session?.id).toBe('a');
  expect(socket.sent).toEqual([{select: ''}, {select: 'a'}]);
  socket.receive({type: 'transcript', session: 'a', reset: true, items: [textItem('1', 1, 'hi')]});
  expect(store.get().session?.transcript.order).toEqual(['1']);
  socket.receive(stateMessage('b1', true));
  expect(socket.sent.at(-1)).toEqual({select: ''});
});

test('it reconnects after a close and selects again', () => {
  started();
  FakeSocket.all[0].open();
  FakeSocket.all[0].receive(stateMessage('b1'));
  FakeSocket.all[0].onclose();
  expect(FakeSocket.all.length).toBe(1);
  jest.advanceTimersByTime(1000);
  expect(FakeSocket.all.length).toBe(2);
  FakeSocket.all[1].open();
  expect(FakeSocket.all[1].sent).toEqual([{select: 'a'}]);
});

test('a reset transcript after a reconnect replaces the old items', () => {
  const store = started();
  const [first] = FakeSocket.all;
  first.open();
  first.receive(stateMessage('b1'));
  first.receive({type: 'transcript', session: 'a', reset: true, items: [textItem('1', 1, 'old'), textItem('2', 2, 'older')]});
  first.onclose();
  jest.advanceTimersByTime(1000);
  const second = FakeSocket.all[1];
  second.open();
  second.receive(stateMessage('b1'));
  second.receive({type: 'transcript', session: 'a', reset: true, items: [textItem('3', 1, 'new')]});
  const transcript = store.get().session?.transcript;
  expect(transcript?.order).toEqual(['3']);
  expect(transcript?.items['3']).toMatchObject({text: 'new'});
});

test('a changed build reloads the page and the first one does not', () => {
  const reload = spyOn(location, 'reload').mockImplementation(() => {});
  started();
  FakeSocket.all[0].open();
  FakeSocket.all[0].receive(stateMessage('b1'));
  FakeSocket.all[0].onclose();
  jest.advanceTimersByTime(1000);
  FakeSocket.all[1].open();
  FakeSocket.all[1].receive(stateMessage('b1'));
  expect(reload).not.toHaveBeenCalled();
  FakeSocket.all[1].receive(stateMessage('b2'));
  expect(reload).toHaveBeenCalledTimes(1);
});

const fileText = (store: Store, path: string) => new Promise<string>(resolve => {
  const unsubscribe = store.subscribe(state => {
    const text = state.session?.files.open[path]?.text;
    if (text === null || text === undefined) return;
    unsubscribe();
    resolve(text);
  });
});

test('an open message keeps and selects a file and reads its contents', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('hello'));
  const store = started();
  const [socket] = FakeSocket.all;
  socket.open();
  socket.receive(stateMessage('b1'));
  socket.receive({type: 'open', session: 'a', path: 'a.go', lines: []});
  expect(store.get().session?.tabs.preview).toBeNull();
  expect(findPane(store.get().session?.tabs.root ?? null, 'file:a.go')?.active).toBe('file:a.go');
  expect(await fileText(store, 'a.go')).toBe('hello');
  expect(fetches).toHaveBeenCalledTimes(1);
});

test('an open received during a session switch waits until the page returns', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('hello'));
  const store = started();
  const [socket] = FakeSocket.all;
  socket.open();
  const message = stateMessage('b1');
  message.workspaces[0].sessions.push(testSession({id: 'b', title: 'B'}));
  socket.receive(message);
  store.update(state => selectSession(state, 'b'));
  socket.receive({type: 'open', session: 'a', path: 'guide.md', lines: [{start: 1, end: 1}]});
  expect(store.get().session?.files.open).toEqual({});
  expect(fetches).not.toHaveBeenCalled();
  store.update(state => selectSession(state, 'a'));
  expect(findPane(store.get().session?.tabs.root ?? null, 'file:guide.md')?.active).toBe('file:guide.md');
  expect(store.get().session?.files.open['guide.md']).toMatchObject({source: true, lines: [{start: 1, end: 1}]});
  expect(store.get().fileOpens).toEqual({});
  expect(await fileText(store, 'guide.md')).toBe('hello');
  expect(fetches).toHaveBeenCalledTimes(1);
  store.update(state => selectSession(state, 'b'));
  store.update(state => selectSession(state, 'a'));
  expect(store.get().session?.files.open['guide.md']).toMatchObject({source: true, reveal: false, text: null});
  expect(fetches).toHaveBeenCalledTimes(1);
});
