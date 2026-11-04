import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from '../shared/icon.jsx';
// A session's Browser tab: its navigation toolbar, and below it the
// body, over which the window shows the session's browser. Enter in the
// address field loads the address, and Escape restores the page's URL.
import {useRef, useState} from 'react';
import {useStore} from '../store.js';
import {inWindow, post} from './browser.js';
import {useBrowserPlacement} from './placement.js';

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
      e.preventDefault();
      setAddress(null);
    }
  };

  return (
    <div className="browser-view flex min-h-0 flex-1 flex-col">
      <header className="browser-view-header flex shrink-0 items-center gap-2 border-b p-2">
        <Button variant="ghost" size="icon" title="Back" aria-label="Back" disabled={!page.canGoBack} onClick={() => send('back')}><Icon name="arrow-left" /></Button>
        <Button variant="ghost" size="icon" title="Forward" aria-label="Forward" disabled={!page.canGoForward} onClick={() => send('forward')}><Icon name="arrow-right" /></Button>
        <Button variant="ghost" size="icon" title="Reload" aria-label="Reload" onClick={() => send('reload')}><Icon name="refresh" /></Button>
        <Input
          className="browser-view-address flex-1"
          aria-label="Address"
          value={address ?? page.url}
          onChange={e => setAddress(e.currentTarget.value)}
          onKeyDown={key}
        />
        <Button variant="ghost" size="icon" title="Open in browser" aria-label="Open in browser" onClick={() => send('external')}><Icon name="link-external" /></Button>
      </header>
      <div ref={body} className="browser-view-body min-h-0 flex-1" aria-busy={page.loading} />
    </div>
  );
}

export function BrowserView({session}) {
  if (!inWindow()) return <div className="browser-view-message m-auto p-6 text-center text-muted-foreground">The browser needs the Hypercube window.</div>;
  return <Page session={session} />;
}
