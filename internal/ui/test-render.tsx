// React roots render synchronously; tests wrap updates in act.
import {mock, spyOn, type Mock} from 'bun:test';
import {act, useLayoutEffect, useState, type ReactNode} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {StoreContext, type Store} from './web/store.ts';

const roots = new Map<Element, Root>();

// render draws element into container, or unmounts it when element is null.
export function render(element: ReactNode | null, container: Element) {
  if (element === null) {
    const root = roots.get(container);
    if (root) act(() => root.unmount());
    roots.delete(container);
    return;
  }
  const root = roots.get(container) ?? createRoot(container);
  roots.set(container, root);
  act(() => flushSync(() => root.render(element)));
}

function Provider({store, children}: {store: Store; children: ReactNode}) {
  const [state, setState] = useState(store.get());
  useLayoutEffect(() => store.subscribe(setState), [store]);
  return <StoreContext.Provider value={[state, store]}>{children}</StoreContext.Provider>;
}

export function mount(store: Store, element: ReactNode): HTMLDivElement {
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
export function setValue(field: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const prototype = field instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  Reflect.set(prototype, 'value', value, field);
}

export function clickControl(element: HTMLElement) {
  act(() => {
    element.dispatchEvent(new PointerEvent('pointerdown', {bubbles: true, cancelable: true, button: 0, pointerType: 'mouse', ctrlKey: false}));
    element.dispatchEvent(new MouseEvent('mousedown', {bubbles: true, cancelable: true, button: 0, ctrlKey: false}));
    element.dispatchEvent(new PointerEvent('pointerup', {bubbles: true, cancelable: true, button: 0, pointerType: 'mouse'}));
    element.dispatchEvent(new MouseEvent('mouseup', {bubbles: true, button: 0}));
    element.click();
  });
}

export type FetchHandler = (url: string, init?: RequestInit) => Response | Promise<Response>;

// stubFetch answers every fetch with handler, which receives each request's
// URL as a string, until mock.restore().
export function stubFetch(handler: FetchHandler): Mock<FetchHandler> {
  const stub = mock(handler);
  spyOn(globalThis, 'fetch').mockImplementation(Object.assign(
    async (input: RequestInfo | URL, init?: RequestInit) => stub(input instanceof Request ? input.url : String(input), init),
    {preconnect: () => {}},
  ));
  return stub;
}

// element returns the first element in root that matches selector and is a
// kind, and fails the test when there is none.
export function element<T extends Element>(root: ParentNode, selector: string, kind: new () => T): T {
  const found = root.querySelector(selector);
  if (!(found instanceof kind)) throw new Error(`No ${kind.name} matches ${selector}`);
  return found;
}

export const find = (root: ParentNode, selector: string) => element(root, selector, HTMLElement);

export const findAll = (root: ParentNode, selector: string) => [...root.querySelectorAll<HTMLElement>(selector)];
