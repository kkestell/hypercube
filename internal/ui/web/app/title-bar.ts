// The window's title bar, which the page draws as the top row of its tab
// bars. The window reports the title bar's height and the width of its buttons
// in a hypercube-title-bar event, {height, inset}, and state.titleBar keeps
// it; a page outside the window has none. The row's empty space drags the
// window, and a double click zooms it, as on a title bar.
import type {CSSProperties} from 'react';
import type {Store} from '../store.ts';
import type {TitleBar} from './types.ts';
import {hypercube} from './window.ts';

export function startTitleBar(store: Store) {
  const bridge = hypercube();
  if (!bridge) return;
  window.addEventListener('hypercube-title-bar', e => store.update(state => ({...state, titleBar: e.detail})));
  bridge.postMessage({type: 'title-bar'});
}

// titleBarVars publishes the title bar's height and the width of the window's
// buttons as the CSS variables the top-row bars size themselves by.
export function titleBarVars(titleBar: TitleBar | null): CSSProperties | undefined {
  if (!titleBar?.height) return undefined;
  return {'--title-bar-height': titleBar.height + 'px', '--title-bar-inset': titleBar.inset + 'px'} as CSSProperties;
}

// topRow is the class of a tab bar in the window's top row: its padding, at
// least the title bar's height, and the drag region that moves the window
// except over its controls. topRowStart also clears the window's buttons, for
// the bar at the window's left edge.
export const topRow = 'min-h-[var(--title-bar-height,0px)] py-2 [app-region:drag] [&_:is(button,input,[role=tab],[draggable=true])]:[app-region:no-drag]';
export const topRowStart = 'pl-[var(--title-bar-inset,0.5rem)]';

// Keep tab controls compact enough to fit the native title-bar row.
export const barTabs = 'group-data-[orientation=horizontal]/tabs:h-7';
