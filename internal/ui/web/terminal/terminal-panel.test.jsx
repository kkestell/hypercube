import {afterEach, beforeEach, expect, jest, mock, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';

class FakeSocket {
  static OPEN = 1;
  static all = [];
  constructor(url) {
    this.url = String(url);
    this.readyState = 0;
    this.closed = false;
    FakeSocket.all.push(this);
  }
  send() {}
  close() { this.closed = true; }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen();
  }
}

let terminals;
mock.module('./xterm.js', () => ({
  createTerminal() {
    const term = {
      cols: 80, rows: 24, resets: 0, disposed: false,
      onData() {}, onBinary() {}, onResize() {},
      reset() { this.resets++; },
      write() {}, focus() {},
      dispose() { this.disposed = true; },
    };
    terminals.push(term);
    return {term, fit: {fit() {}}};
  },
}));
const {TerminalPanel} = await import('./terminal-panel.jsx');

const realSocket = globalThis.WebSocket;
beforeEach(() => {
  terminals = [];
  FakeSocket.all = [];
  globalThis.WebSocket = FakeSocket;
  jest.useFakeTimers();
});
afterEach(() => {
  unmountAll();
  globalThis.WebSocket = realSocket;
  jest.useRealTimers();
});

const setup = () => {
  const store = createStore({session: {id: 's'}, workbench: {panel: true}});
  mount(store, <TerminalPanel />);
  return store;
};
const panelShown = store => store.get().workbench.panel;

test('the panel connects to the session\'s shell', () => {
  setup();
  expect(FakeSocket.all.length).toBe(1);
  expect(new URL(FakeSocket.all[0].url).searchParams.get('session')).toBe('s');
});

test('the panel closes when the shell exits', () => {
  const store = setup();
  const [socket] = FakeSocket.all;
  socket.open();
  act(() => {
    socket.onmessage({data: JSON.stringify({exit: 0})});
    socket.onclose();
  });
  expect(panelShown(store)).toBe(false);
  expect(socket.closed).toBe(true);
  expect(terminals[0].disposed).toBe(true);
});

test('the panel closes when the shell cannot connect', () => {
  const store = setup();
  act(() => FakeSocket.all[0].onclose());
  expect(panelShown(store)).toBe(false);
  expect(terminals[0].disposed).toBe(true);
});

test('a dropped connection reconnects and starts the terminal over', () => {
  const store = setup();
  FakeSocket.all[0].open();
  act(() => FakeSocket.all[0].onclose());
  expect(panelShown(store)).toBe(true);
  expect(FakeSocket.all.length).toBe(1);
  act(() => jest.advanceTimersByTime(1000));
  expect(FakeSocket.all.length).toBe(2);
  FakeSocket.all[1].open();
  expect(terminals[0].resets).toBe(2);
  expect(terminals.length).toBe(1);
});
