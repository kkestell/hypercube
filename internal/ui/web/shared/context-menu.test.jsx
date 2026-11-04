import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {ContextMenu} from './context-menu.jsx';

const Observer = globalThis.ResizeObserver;
afterEach(() => {
  unmountAll();
  globalThis.ResizeObserver = Observer;
});

test('a menu near the bottom moves up once it draws its items', () => {
  const resized = [];
  globalThis.ResizeObserver = class {
    constructor(callback) { resized.push(callback); }
    observe() {}
    disconnect() {}
  };
  const store = createStore({overlays: {}});
  const root = mount(store, <ContextMenu x={0} y={innerHeight - 10} items={[]} onClose={() => {}} />);
  const box = root.querySelector('.context-menu');
  expect(box.style.top).toBe(innerHeight - 10 + 'px');
  Object.defineProperty(box, 'offsetHeight', {value: 100});
  act(() => resized.forEach(callback => callback()));
  expect(box.style.top).toBe(innerHeight - 100 + 'px');
});

test('selecting an item returns focus before running its action', () => {
  const invoker = document.createElement('button');
  document.body.append(invoker);
  invoker.focus();
  let focusedDuringAction = false;
  const store = createStore({overlays: {}});
  const root = mount(store, <ContextMenu x={0} y={0} items={[{label: 'Action', run: () => { focusedDuringAction = document.activeElement === invoker; }}]} onClose={() => {}} />);
  root.querySelector('vscode-context-menu').focus();
  act(() => root.querySelector('vscode-context-menu').dispatchEvent(new CustomEvent('vsc-context-menu-select', {detail: {value: '0'}})));
  expect(focusedDuringAction).toBe(true);
});
