import {setValue} from '../../test-render.jsx';
import {afterEach, beforeEach, expect, spyOn, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.jsx';
import {ContextMenu} from '../shared/context-menu.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {BrowserView} from './browser-view.jsx';

let posted, rect;
beforeEach(() => {
  posted = [];
  rect = spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({left: 10, top: 20, right: 410, bottom: 320, width: 400, height: 300});
  globalThis.webkit = {messageHandlers: {hypercube: {postMessage: message => posted.push(message)}}};
});
afterEach(() => {
  unmountAll();
  rect.mockRestore();
  delete globalThis.webkit;
});

const page = {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: true, canGoForward: false};

function setup() {
  const store = createStore({confirmation: null, browsers: {s: page}, session: emptySession('s'), overlays: {}});
  const root = mount(store, <BrowserView session="s" />);
  return {store, root};
}

const places = () => posted.filter(m => m.type === 'place');

test('outside the window the tab says the browser needs it', () => {
  delete globalThis.webkit;
  const {root} = setup();
  expect(root.textContent).toBe('The browser needs the Hypercube window.');
});

test('the toolbar sends its buttons and the entered address to the window', () => {
  const {root} = setup();
  const button = title => root.querySelector(`[title="${title}"]`);
  expect(button('Back').hasAttribute('disabled')).toBe(false);
  expect(button('Forward').hasAttribute('disabled')).toBe(true);
  act(() => button('Back').click());
  act(() => button('Reload').click());
  act(() => button('Open in browser').click());
  const address = root.querySelector('input');
  expect(address.getAttribute('value')).toBe(page.url);
  setValue(address, 'example.org');
  act(() => address.dispatchEvent(new Event('input', {bubbles: true})));
  act(() => address.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(posted.filter(m => m.type !== 'place')).toEqual([
    {type: 'back', session: 's'},
    {type: 'reload', session: 's'},
    {type: 'external', session: 's'},
    {type: 'navigate', session: 's', url: 'example.org'},
  ]);
});

test('Escape restores the address and is consumed before reaching the app', () => {
  const {root} = setup();
  const address = root.querySelector('input');
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
  const hidden = {type: 'place', session: null, rect: null};
  expect(places()).toEqual([shown]);
  act(() => store.update(s => ({...s, confirmation: {message: 'Sure?'}})));
  act(() => store.update(s => ({...s, confirmation: null})));
  expect(places()).toEqual([shown, hidden, shown]);
  const menu = mount(store, <ContextMenu x={0} y={0} items={[]} onClose={() => {}} />);
  expect(places()).toEqual([shown, hidden, shown, hidden]);
  act(() => menu.remove());
  unmountAll();
  expect(places().at(-1)).toEqual(hidden);
});

test('the browser hides once a menu grows over the body', () => {
  const Observer = globalThis.ResizeObserver;
  const resized = new Map();
  globalThis.ResizeObserver = class {
    constructor(callback) { this.callback = callback; }
    observe(element) { resized.set(element, this.callback); }
    disconnect() {}
  };
  let menuRect = {left: 150, top: 0, right: 310, bottom: 20, width: 160, height: 20};
  const bodyRect = rect();
  rect.mockImplementation(function () { return this.classList.contains('context-menu') ? menuRect : bodyRect; });
  try {
    const {store} = setup();
    mount(store, <ContextMenu x={150} y={0} items={[]} onClose={() => {}} />);
    const menu = document.querySelector('.context-menu');
    expect(places().at(-1).session).toBe('s');
    menuRect = {...menuRect, bottom: 70, height: 70};
    act(() => resized.get(menu)([{target: menu, borderBoxSize: [{inlineSize: menuRect.width, blockSize: menuRect.height}], contentRect: menuRect}]));
    expect(places().at(-1)).toEqual({type: 'place', session: null, rect: null});
  } finally {
    globalThis.ResizeObserver = Observer;
  }
});
