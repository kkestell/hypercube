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
