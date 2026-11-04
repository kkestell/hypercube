// The window's title bar, which the page draws as its header. The window
// reports the title bar's height and the width of its buttons in a
// hypercube-title-bar event, {height, inset}, and state.titleBar keeps it; a
// page outside the window has none. A mouse down on the header outside its
// buttons moves the window, and a double click zooms it, as on a title bar.
import type {CSSProperties, MouseEvent} from 'react';
import type {Store} from '../store.ts';
import type {TitleBar} from './types.ts';
import {windowHandler} from './window.ts';

export function startTitleBar(store: Store) {
  const handler = windowHandler();
  if (!handler) return;
  window.addEventListener('hypercube-title-bar', e => store.update(state => ({...state, titleBar: e.detail})));
  handler.postMessage({type: 'title-bar'});
}

export function titleBarMouseDown(e: MouseEvent) {
  const handler = windowHandler();
  if (!handler || e.button !== 0 || !(e.target instanceof Element) || e.target.closest('button')) return;
  e.preventDefault();
  handler.postMessage({type: e.detail === 2 ? 'zoom' : 'drag'});
}

// titleBarStyle fits the header to the title bar, beside the window's buttons.
export function titleBarStyle(titleBar: TitleBar | null): CSSProperties | undefined {
  if (!titleBar?.height) return undefined;
  return {height: titleBar.height + 'px', paddingLeft: titleBar.inset + 'px'};
}
