import {afterEach, beforeEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {element, mount, render, setValue, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {ContextMenu} from '../shared/context-menu.tsx';
import {createStore} from '../store.ts';
import {BrowserView} from './browser-view.tsx';
import type {BrowserPage} from './types.ts';

let posted: object[];
const bodyRect = new DOMRect(10, 20, 400, 300);
const stubRects = (rectOf: (target: HTMLElement) => DOMRect) =>
  spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) { return rectOf(this); });
const realResizeObserver = globalThis.ResizeObserver;

beforeEach(() => {
  posted = [];
  stubRects(() => bodyRect);
  globalThis.webkit = {messageHandlers: {hypercube: {postMessage: message => posted.push(message)}}};
});
afterEach(() => {
  unmountAll();
  mock.restore();
  Object.defineProperty(globalThis, 'ResizeObserver', {configurable: true, writable: true, value: realResizeObserver});
  delete globalThis.webkit;
});

const page: BrowserPage = {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: true, canGoForward: false};

function setup() {
  const store = createStore(testState({browsers: {s: page}, session: emptySession('s')}));
  const root = mount(store, <BrowserView session="s" />);
  return {store, root};
}

const places = () => posted.filter(m => 'type' in m && m.type === 'place');
const hidden = {type: 'place', session: null, rect: null};

test('outside the window the tab says the browser needs it', () => {
  delete globalThis.webkit;
  const {root} = setup();
  expect(root.textContent).toBe('The browser needs the Hypercube window.');
});

test('the toolbar sends its buttons and the entered address to the window', () => {
  const {root} = setup();
  const button = (title: string) => element(root, `[title="${title}"]`, HTMLButtonElement);
  expect(button('Back').disabled).toBe(false);
  expect(button('Forward').disabled).toBe(true);
  act(() => button('Back').click());
  act(() => button('Reload').click());
  act(() => button('Open in browser').click());
  const address = element(root, 'input', HTMLInputElement);
  expect(address.getAttribute('value')).toBe(page.url);
  setValue(address, 'example.org');
  act(() => address.dispatchEvent(new Event('input', {bubbles: true})));
  act(() => address.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(posted.filter(m => !('type' in m && m.type === 'place'))).toEqual([
    {type: 'back', session: 's'},
    {type: 'reload', session: 's'},
    {type: 'external', session: 's'},
    {type: 'navigate', session: 's', url: 'example.org'},
  ]);
});

test('Escape restores the address and is consumed before reaching the app', () => {
  const {root} = setup();
  const address = element(root, 'input', HTMLInputElement);
  setValue(address, 'draft');
  act(() => address.dispatchEvent(new Event('input', {bubbles: true})));
  let prevented = false;
  root.addEventListener('keydown', e => { prevented = e.defaultPrevented; });
  act(() => address.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(prevented).toBe(true);
  expect(address.getAttribute('value')).toBe(page.url);
});

test('the browser shows over the body, and hides under the dialog, a menu, and once the tab closes', () => {
  const {store} = setup();
  const shown = {type: 'place', session: 's', rect: {x: 10, y: 20, width: 400, height: 300}};
  expect(places()).toEqual([shown]);
  act(() => store.update(s => ({...s, confirmation: {message: 'Sure?', detail: '', primary: 'OK', accept: () => {}}})));
  act(() => store.update(s => ({...s, confirmation: null})));
  expect(places()).toEqual([shown, hidden, shown]);
  const menu = mount(store, <ContextMenu x={0} y={0} items={[]} onClose={() => {}} />);
  expect(places()).toEqual([shown, hidden, shown, hidden]);
  act(() => menu.remove());
  unmountAll();
  expect(places().at(-1)).toEqual(hidden);
});

test('the browser hides once a menu grows over the body', () => {
  const resized = new Map<Element, () => void>();
  class TestResizeObserver implements ResizeObserver {
    constructor(private callback: ResizeObserverCallback) {}
    observe(target: Element) { resized.set(target, () => this.callback([], this)); }
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {configurable: true, writable: true, value: TestResizeObserver});
  let menuRect = new DOMRect(150, 0, 160, 20);
  stubRects(target => target.classList.contains('context-menu') ? menuRect : bodyRect);
  const {store} = setup();
  mount(store, <ContextMenu x={150} y={0} items={[]} onClose={() => {}} />);
  const menu = element(document, '.context-menu', HTMLElement);
  expect(places().at(-1)).toMatchObject({session: 's'});
  menuRect = new DOMRect(150, 0, 160, 70);
  act(() => resized.get(menu)?.());
  expect(places().at(-1)).toEqual(hidden);
});

test('a modal hides the browser in another pane until it closes', () => {
  const dialogRect = new DOMRect(500, 350, 200, 150);
  stubRects(target => target.getAttribute('role') === 'dialog' ? dialogRect : bodyRect);
  const {store} = setup();
  expect(places().at(-1)).toMatchObject({session: 's'});
  const root = mount(store, <CommandDialog open onOpenChange={() => {}} title="Search" description="Search files">{null}</CommandDialog>);
  expect(places().at(-1)).toEqual(hidden);
  render(null, root);
  expect(places().at(-1)).toEqual({type: 'place', session: 's', rect: {x: 10, y: 20, width: 400, height: 300}});
});
