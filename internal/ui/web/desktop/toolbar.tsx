// The toolbar above an agent's browser page in the desktop app: back, forward,
// reload, the address, and open in the default browser. The app reports the
// page's state and carries out the controls.
import {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {startAppearance} from '../app/appearance.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from '../shared/icon.tsx';

const bridge = globalThis.hypercubeBrowser!;

function Toolbar() {
  const [state, setState] = useState<BrowserState>({url: '', canGoBack: false, canGoForward: false});
  // typed is an address being edited, which the page's changes do not replace.
  const [typed, setTyped] = useState<string | null>(null);
  useEffect(() => bridge.onState(setState), []);
  return <form className="flex h-full items-center gap-1 px-2" onSubmit={e => {
    e.preventDefault();
    if (typed !== null) bridge.navigate(typed);
    setTyped(null);
  }}>
    <Button type="button" variant="ghost" size="icon-sm" aria-label="Back" title="Back" disabled={!state.canGoBack} onClick={bridge.back}><Icon name="arrow-left" /></Button>
    <Button type="button" variant="ghost" size="icon-sm" aria-label="Forward" title="Forward" disabled={!state.canGoForward} onClick={bridge.forward}><Icon name="arrow-right" /></Button>
    <Button type="button" variant="ghost" size="icon-sm" aria-label="Reload" title="Reload" onClick={bridge.reload}><Icon name="refresh" /></Button>
    <Input className="h-8" aria-label="Address" placeholder="Address" value={typed ?? state.url}
      onChange={e => setTyped(e.target.value)} onBlur={() => setTyped(null)}
      onKeyDown={e => { if (e.key === 'Escape') { setTyped(null); bridge.focusSite(); } }} />
    <Button type="button" variant="ghost" size="icon-sm" aria-label="Open in default browser" title="Open in default browser" onClick={bridge.openExternal}><Icon name="link-external" /></Button>
  </form>;
}

startAppearance();
createRoot(document.getElementById('app')!).render(<Toolbar />);
