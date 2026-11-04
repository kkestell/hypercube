// The window's title bar, which the page draws as its header. The window
// reports the title bar's height and the width of its buttons in a
// hypercube-title-bar event, {height, inset}, and state.titleBar keeps it; a
// page outside the window has none. A mouse down on the header outside its
// buttons moves the window, and a double click zooms it, as on a title bar.
import {windowHandler} from './window.js';

export function startTitleBar(store) {
  if (!windowHandler()) return;
  addEventListener('hypercube-title-bar', e => store.update(state => ({...state, titleBar: e.detail})));
  windowHandler().postMessage({type: 'title-bar'});
}

export function titleBarMouseDown(e) {
  if (!windowHandler() || e.button !== 0 || e.target.closest('vscode-toolbar-button')) return;
  e.preventDefault();
  windowHandler().postMessage({type: e.detail === 2 ? 'zoom' : 'drag'});
}

// titleBarStyle fits the header to the title bar, beside the window's buttons.
export function titleBarStyle(titleBar) {
  if (!titleBar?.height) return undefined;
  return {height: titleBar.height + 'px', paddingLeft: titleBar.inset + 'px'};
}
