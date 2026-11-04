// React roots and store updates stay synchronous for behavior tests.
import {act, useLayoutEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {StoreContext} from './web/store.js';

const roots = new Map();

export function render(element, container) {
  if (element === null) {
    const root = roots.get(container);
    if (root) act(() => root.unmount());
    roots.delete(container);
    return;
  }
  let root = roots.get(container);
  if (!root) {
    root = createRoot(container);
    roots.set(container, root);
  }
  act(() => flushSync(() => root.render(element)));
}

function Provider({store, children}) {
  const [state, setState] = useState(store.get());
  useLayoutEffect(() => store.subscribe(next => act(() => setState(next))), [store]);
  return <StoreContext.Provider value={[state, store]}>{children}</StoreContext.Provider>;
}

export function mount(store, element) {
  const root = document.createElement('div');
  document.body.append(root);
  act(() => render(<Provider store={store}>{element}</Provider>, root));
  return root;
}

export function unmountAll() {
  for (const [container] of roots) render(null, container);
  document.body.replaceChildren();
}

// React tracks assignments to value; typing changes the underlying DOM value.
export function setValue(field, value) {
  const prototype = field instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, 'value').set.call(field, value);
}

export function clickControl(element) {
  act(() => {
    element.dispatchEvent(new PointerEvent('pointerdown', {bubbles: true, cancelable: true, button: 0, pointerType: 'mouse', ctrlKey: false}));
    element.dispatchEvent(new MouseEvent('mousedown', {bubbles: true, cancelable: true, button: 0, ctrlKey: false}));
    element.dispatchEvent(new PointerEvent('pointerup', {bubbles: true, cancelable: true, button: 0, pointerType: 'mouse'}));
    element.dispatchEvent(new MouseEvent('mouseup', {bubbles: true, button: 0}));
    element.click();
  });
}
