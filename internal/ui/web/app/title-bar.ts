// The window's title bar, which the page draws as the top row of its tab
// bars. The window reports the title bar's height and the width of its buttons
// in a hypercube-title-bar event, {height, inset}, and state.titleBar keeps
// it; a page outside the window has none. A mouse down on a top-row bar
// outside its buttons moves the window, and a double click zooms it, as on a
// title bar.
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

// titleBarVars publishes the title bar's height and the width of the window's
// buttons as the CSS variables the top-row bars size themselves by.
export function titleBarVars(titleBar: TitleBar | null): CSSProperties | undefined {
  if (!titleBar?.height) return undefined;
  return {'--title-bar-height': titleBar.height + 'px', '--title-bar-inset': titleBar.inset + 'px'} as CSSProperties;
}

// topRow is the class of a tab bar in the window's top row, as tall as the
// title bar. topRowStart also clears the window's buttons, for the bar at the
// window's left edge.
export const topRow = 'min-h-[var(--title-bar-height,0px)]';
export const topRowStart = 'pl-[var(--title-bar-inset,0.5rem)]';

// barButton and barTabs size the controls of a top-row bar, which is only a
// little taller than they are.
export const barButton = 'size-7';
export const barTabs = 'group-data-[orientation=horizontal]/tabs:h-7';
