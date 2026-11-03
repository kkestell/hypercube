// Each session's browser: a web page that the Hypercube window shows over the
// session's Browser tab. The page reaches the window through its hypercube
// message handler, which a page outside the window lacks. The window reports
// a browser's state in a hypercube-browser event, {session, url, title,
// loading, canGoBack, canGoForward, reveal}, and state.browsers keeps it by
// session. The attached session's Browser tab opens when the page first
// learns of its browser and whenever its agent navigates it.
import {openBrowserTab} from '../editor/editor.js';

const handler = () => globalThis.webkit?.messageHandlers?.hypercube;

export const inWindow = () => !!handler();

export const post = message => handler().postMessage(message);

// receivedBrowser applies a browser's state; reveal selects its tab.
export function receivedBrowser(state, {session, reveal, ...page}) {
  const known = !!state.browsers[session];
  const next = {...state, browsers: {...state.browsers, [session]: page}};
  if (next.session?.id !== session || (known && !reveal)) return next;
  return openBrowserTab(next, reveal);
}

// startBrowsers listens for the window's reports and asks for every
// browser's state.
export function startBrowsers(store) {
  if (!inWindow()) return;
  addEventListener('hypercube-browser', e => store.update(state => receivedBrowser(state, e.detail)));
  post({type: 'ready'});
}
