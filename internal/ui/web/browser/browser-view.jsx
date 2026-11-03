// A session's Browser tab: VS Code's Simple Browser toolbar, and below it the
// body, over which the window shows the session's browser. Enter in the
// address field loads the address, and Escape restores the page's URL.
import {useRef, useState} from 'preact/hooks';
import {useStore} from '../store.js';
import {inWindow, post} from './browser.js';
import {useBrowserPlacement} from './placement.js';
import './browser-view.css';

const blank = {url: '', title: '', loading: false, canGoBack: false, canGoForward: false};

function Page({session}) {
  const [state] = useStore();
  const body = useRef(null);
  const [address, setAddress] = useState(null);
  const page = state.browsers[session] ?? blank;
  useBrowserPlacement(body, session, !!state.confirmation || !!state.session.tabs.dragging);

  const send = (type, fields) => post({type, session, ...fields});
  const key = e => {
    if (e.key === 'Enter') {
      send('navigate', {url: e.currentTarget.value});
      setAddress(null);
    } else if (e.key === 'Escape') {
      setAddress(null);
    }
  };

  return (
    <div class="browser-view">
      <header class="browser-view-header">
        <nav class="browser-view-controls">
          <button type="button" title="Back" disabled={!page.canGoBack} onClick={() => send('back')}><vscode-icon name="arrow-left" /></button>
          <button type="button" title="Forward" disabled={!page.canGoForward} onClick={() => send('forward')}><vscode-icon name="arrow-right" /></button>
          <button type="button" title="Reload" onClick={() => send('reload')}><vscode-icon name="refresh" /></button>
        </nav>
        <input
          class="browser-view-address"
          type="text"
          aria-label="Address"
          value={address ?? page.url}
          onInput={e => setAddress(e.currentTarget.value)}
          onKeyDown={key}
        />
        <nav class="browser-view-controls">
          <button type="button" title="Open in browser" onClick={() => send('external')}><vscode-icon name="link-external" /></button>
        </nav>
      </header>
      <div ref={body} class="browser-view-body" aria-busy={page.loading} />
    </div>
  );
}

export function BrowserView({session}) {
  if (!inWindow()) return <div class="browser-view-message">The browser needs the Hypercube window.</div>;
  return <Page session={session} />;
}
