import {afterEach, beforeEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
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
  const store = createStore({confirmation: null, browsers: {s: page}, session: emptySession('s')});
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
  const button = title => root.querySelector(`button[title="${title}"]`);
  expect(button('Back').disabled).toBe(false);
  expect(button('Forward').disabled).toBe(true);
  act(() => button('Back').click());
  act(() => button('Reload').click());
  act(() => button('Open in browser').click());
  const address = root.querySelector('input');
  expect(address.value).toBe(page.url);
  address.value = 'example.org';
  act(() => address.dispatchEvent(new Event('input')));
  act(() => address.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  expect(posted.filter(m => m.type !== 'place')).toEqual([
    {type: 'back', session: 's'},
    {type: 'reload', session: 's'},
    {type: 'external', session: 's'},
    {type: 'navigate', session: 's', url: 'example.org'},
  ]);
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
