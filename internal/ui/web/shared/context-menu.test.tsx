import {afterEach, expect, test} from 'bun:test';
import {useState} from 'react';
import {clickControl, find, mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {ContextMenu} from './context-menu.tsx';

afterEach(unmountAll);


test('a menu uses stock menu items and reports its bounds for browser placement', () => {
  const store = createStore(testState());
  mount(store, <ContextMenu x={0} y={innerHeight - 10} items={[[{label: 'Action', icon: 'check', run() {}}]]} onClose={() => {}} />);
  expect(find(document, '[role=menuitem]').textContent).toBe('Action');
  expect(Object.keys(store.get().overlays)).toHaveLength(1);
  unmountAll();
  expect(Object.keys(store.get().overlays)).toHaveLength(0);
});

test('selecting an item returns focus before running its action', () => {
  const invoker = document.createElement('button');
  document.body.append(invoker);
  invoker.focus();
  let focusedDuringAction = false;
  const store = createStore(testState());
  function Menu() {
    const [open, setOpen] = useState(true);
    return open && <ContextMenu x={0} y={0} items={[[{label: 'Action', icon: 'check', run: () => { focusedDuringAction = document.activeElement === invoker; }}]]} onClose={() => setOpen(false)} />;
  }
  mount(store, <Menu />);
  clickControl(find(document, '[role=menuitem]'));
  expect(focusedDuringAction).toBe(true);
});
