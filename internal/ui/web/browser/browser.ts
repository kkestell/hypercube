// Each session's browser: a web page that the Hypercube window shows over the
// session's Browser tab. The page reaches the window through its hypercube
// message handler, which a page outside the window lacks. The window reports
// a browser's state in a hypercube-browser event, {session, url, title,
// loading, canGoBack, canGoForward, reveal}, and state.browsers keeps it by
// session. The attached session's Browser tab opens when the page first
// learns of its browser and whenever its agent navigates it. When the agent
// closes its browser, the window sends a hypercube-browser-closed event,
// {session}, and the tab closes.
import {browserTabId, closeTab, openBrowserTab} from '../editor/editor.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {BrowserMessage, BrowserReport} from './types.ts';

const handler = () => globalThis.webkit?.messageHandlers.hypercube;

export const inWindow = () => !!handler();

export const post = (message: BrowserMessage) => handler()?.postMessage(message);

// receivedBrowser applies a browser's state; reveal selects its tab.
export function receivedBrowser(state: State, {session, reveal, ...page}: BrowserReport): State {
  const known = !!state.browsers[session];
  const next = {...state, browsers: {...state.browsers, [session]: page}};
  if (next.session?.id !== session || (known && !reveal)) return next;
  return openBrowserTab(next, reveal);
}

// closedBrowser forgets a closed browser and closes its tab.
export function closedBrowser(state: State, {session}: {session: string}): State {
  const browsers = {...state.browsers};
  delete browsers[session];
  const next = {...state, browsers};
  return next.session?.id === session ? closeTab(next, browserTabId) : next;
}

// startBrowsers listens for the window's reports and asks for every
// browser's state.
export function startBrowsers(store: Store): void {
  if (!inWindow()) return;
  window.addEventListener('hypercube-browser', e => store.update(state => receivedBrowser(state, e.detail)));
  window.addEventListener('hypercube-browser-closed', e => store.update(state => closedBrowser(state, e.detail)));
  post({type: 'ready'});
}
