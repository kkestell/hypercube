import {afterEach, beforeEach, expect, jest, mock, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.tsx';
import {testConfig, testState} from '../../test-state.ts';
import type {TerminalStyle} from './xterm.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore, type Store} from '../store.ts';

// FakeSocket stands in for the WebSocket that TerminalPanel opens.
class FakeSocket {
  static OPEN = 1;
  static all: FakeSocket[] = [];
  url: string;
  readyState = 0;
  closed = false;
  binaryType = '';
  onopen: () => void = () => {};
  onmessage: (event: {data: string | ArrayBuffer}) => void = () => {};
  onclose: () => void = () => {};
  constructor(url: string | URL) {
    this.url = String(url);
    FakeSocket.all.push(this);
  }
  send() {}
  close() { this.closed = true; }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen();
  }
}

class FakeTerminal {
  cols = 80;
  rows = 24;
  resets = 0;
  styles: TerminalStyle[] = [];
  disposed = false;
  onData() {}
  onBinary() {}
  onResize() {}
  reset() { this.resets++; }
  write() {}
  focus() {}
  dispose() { this.disposed = true; }
}

let terminals: FakeTerminal[];
mock.module('./xterm.ts', () => ({
  createTerminal(_box: HTMLElement, style: TerminalStyle) {
    const term = new FakeTerminal();
    term.styles.push(style);
    terminals.push(term);
    return {term, fit: {fit() {}}, restyle: (style: TerminalStyle) => term.styles.push(style), dispose: () => term.dispose()};
  },
}));
const {TerminalPanel} = await import('./terminal-panel.tsx');

const realSocket = globalThis.WebSocket;
const useSocket = (socket: typeof WebSocket | typeof FakeSocket) => Object.defineProperty(globalThis, 'WebSocket', {value: socket, configurable: true, writable: true});
beforeEach(() => {
  terminals = [];
  FakeSocket.all = [];
  useSocket(FakeSocket);
  jest.useFakeTimers();
});
afterEach(() => {
  unmountAll();
  useSocket(realSocket);
  jest.useRealTimers();
});

const setup = () => {
  const initial = testState();
  const store = createStore({...initial, session: emptySession('s'), workbench: {...initial.workbench, panel: true}});
  mount(store, <TerminalPanel />);
  return store;
};
const panelShown = (store: Store) => store.get().workbench.panel;

test('the panel connects to the session\'s shell', () => {
  setup();
  expect(FakeSocket.all.length).toBe(1);
  expect(new URL(FakeSocket.all[0].url).searchParams.get('session')).toBe('s');
});

test('the open terminal follows the theme and text size settings', () => {
  const store = setup();
  act(() => store.update(state => ({...state, config: testConfig({theme: 'dark', terminal_font_size: 16})})));
  expect(terminals[0].styles.at(0)).toEqual({dark: false, fontSize: 13});
  expect(terminals[0].styles.at(-1)).toEqual({dark: true, fontSize: 16});
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
