// The page frame: the sidebar, the center with the terminal panel below it,
// the right panel, and the status bar, with the notification cards and the
// confirmation dialog over them. App draws from the store's state,
// keeps the page title and URL hash in step with it, and refuses drops that
// would land outside their targets: a tab outside the tabbed area could type
// its ID into a text box, and a file outside an agent pane would open in the
// browser.
import {useEffect, useLayoutEffect, useMemo, useState} from 'preact/hooks';
import {post} from '../api.js';
import {NotificationStack} from '../notifications/notification-stack.jsx';
import {currentSession, pageTitle} from '../sessions/selection.js';
import {SessionView} from '../sessions/session-view.jsx';
import {ConfirmDialog} from '../shared/confirm-dialog.jsx';
import {Split} from '../shared/split.jsx';
import {Sidebar} from '../sidebar/sidebar.jsx';
import {StoreContext} from '../store.js';
import {TerminalPanel} from '../terminal/terminal-panel.jsx';
import {LayoutToggles} from './layout-toggles.jsx';
import {RightPanel} from './right-panel.jsx';
import {StatusBar} from './status-bar.jsx';
import {titleBarMouseDown, titleBarStyle} from './title-bar.js';
import './app.css';

function refuseStrayDrops(store) {
  const refuse = e => {
    const draggingTab = store.get().session?.tabs.dragging && !e.target.closest?.('.editor-area');
    const strayFile = e.dataTransfer?.types.includes('Files') && !e.defaultPrevented;
    if (!draggingTab && !strayFile) return;
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'none';
  };
  addEventListener('dragover', refuse);
  addEventListener('drop', refuse);
  return () => {
    removeEventListener('dragover', refuse);
    removeEventListener('drop', refuse);
  };
}

export function App({store}) {
  const [state, setState] = useState(store.get());
  useLayoutEffect(() => {
    const unsubscribe = store.subscribe(setState);
    setState(store.get());
    return unsubscribe;
  }, [store]);
  useEffect(() => refuseStrayDrops(store), []);
  const context = useMemo(() => [state, store], [state]);

  const title = pageTitle(state);
  useEffect(() => {
    document.title = title;
  }, [title]);
  useEffect(() => {
    const selected = state.selected ?? '';
    if (decodeURIComponent(location.hash.slice(1)) !== selected) {
      history.replaceState(null, '', selected ? '#' + encodeURIComponent(selected) : location.pathname);
    }
  }, [state.selected]);

  const {sidebar, rightPanel, panel} = state.workbench;
  const key = e => {
    if (e.key !== 'Escape' || e.defaultPrevented || e.repeat || e.isComposing || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    // Escape belongs to the terminal, dialogs, and menus while they have focus.
    if (state.confirmation || e.target.closest('.context-menu, .terminal-panel')) return;
    const session = currentSession(state);
    if (!session?.running || session.ended || session.activity !== 'busy' || state.unsent) return;
    e.preventDefault();
    post(store, 'session', {action: 'interrupt', id: session.id});
  };
  return (
    <StoreContext.Provider value={context}>
      <div class="app" onKeyDown={key}>
        <header class="app-header" style={titleBarStyle(state.titleBar)} onMouseDown={titleBarMouseDown}>
          <LayoutToggles />
        </header>
        <div class="app-body">
          <Split fixed="start" size={260} minStart="160px" minEnd="50%" collapsed={sidebar ? null : 'start'}>
            <Sidebar />
            <Split fixed="end" size={320} minStart="60%" collapsed={rightPanel ? null : 'end'}>
              <Split split="horizontal" fixed="end" size={240} minStart="20%" minEnd="40px" collapsed={panel ? null : 'end'}>
                <main class="app-center"><SessionView key={state.session?.id ?? (state.unsent ? 'unsent' : '')} /></main>
                <TerminalPanel key={state.session?.id ?? ''} />
              </Split>
              <RightPanel />
            </Split>
          </Split>
        </div>
        <StatusBar />
        <NotificationStack />
        <ConfirmDialog />
      </div>
    </StoreContext.Provider>
  );
}
