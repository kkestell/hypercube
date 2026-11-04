import {afterEach, expect, test} from 'bun:test';
import {useState} from 'react';
import {clickControl, mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {ContextMenu} from './context-menu.jsx';

afterEach(unmountAll);

test('a menu uses stock menu items and reports its bounds for browser placement', () => {
  const store = createStore({overlays: {}});
  mount(store, <ContextMenu x={0} y={innerHeight - 10} items={[{label: 'Action', run() {}}]} onClose={() => {}} />);
  expect(document.querySelector('[role=menuitem]').textContent).toBe('Action');
  expect(Object.keys(store.get().overlays)).toHaveLength(1);
  unmountAll();
  expect(Object.keys(store.get().overlays)).toHaveLength(0);
});

test('selecting an item returns focus before running its action', () => {
  const invoker = document.createElement('button');
  document.body.append(invoker);
  invoker.focus();
  let focusedDuringAction = false;
  const store = createStore({overlays: {}});
  function Menu() {
    const [open, setOpen] = useState(true);
    return open && <ContextMenu x={0} y={0} items={[{label: 'Action', run: () => { focusedDuringAction = document.activeElement === invoker; }}]} onClose={() => setOpen(false)} />;
  }
  mount(store, <Menu />);
  clickControl(document.querySelector('[role=menuitem]'));
  expect(focusedDuringAction).toBe(true);
});
