// The page frame: the sidebar, the center with the terminal panel below it,
// the right panel, and the status bar, with the notification cards and the
// confirmation dialog over them. App draws from the store's state,
// keeps the page title and URL hash in step with it, and refuses drops that
// would land outside their targets: a tab outside the tabbed area could type
// its ID into a text box, and a file outside an agent pane would open in the
// browser.
import {useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent} from 'react';
import {post} from '../api.ts';
import {focusComposerField} from '../agent/focus-composer.ts';
import {agentTabId, selectTab} from '../editor/editor.ts';
import {findPane} from '../editor/tab-layout.ts';
import {QuickOpen} from '../files/quick-open.tsx';
import {NotificationStack} from '../notifications/notification-stack.tsx';
import {currentSession, pageTitle} from '../sessions/selection.ts';
import {SessionView} from '../sessions/session-view.tsx';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {Split} from '../shared/split.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import {Sidebar} from '../sidebar/sidebar.tsx';
import type {State} from '../state.ts';
import {StoreContext, type Store} from '../store.ts';
import {TerminalPanel} from '../terminal/terminal-panel.tsx';
import {RightPanel} from './right-panel.tsx';
import {binding, matches, nativeShortcut, shortcut, shortcuts, type ShortcutActions} from './shortcuts.ts';
import {SettingsDialog} from './settings-dialog.tsx';
import {ShortcutsDialog} from './shortcuts-dialog.tsx';
import {windowHandler} from './window.ts';
import {titleBarVars} from './title-bar.ts';

function refuseStrayDrops(store: Store) {
  const refuse = (e: DragEvent) => {
    const draggingTab = store.get().session?.tabs.dragging && !(e.target instanceof Element && e.target.closest('.editor-area'));
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

export function App({store}: {store: Store}) {
  const [state, setState] = useState(store.get());
  const [quickOpen, setQuickOpen] = useState(false);
  const [sessionSearch, setSessionSearch] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const focusPending = useRef(false);
  useLayoutEffect(() => {
    if (!focusPending.current) return;
    focusPending.current = false;
    focusComposerField();
  }, [state]);
  const focusComposer = () => {
    if (focusComposerField()) return;
    const session = store.get().session;
    const pane = session && findPane(session.tabs.root, agentTabId);
    if (!pane) return;
    focusPending.current = true;
    store.update(s => selectTab(s, pane.id, agentTabId));
  };
  useLayoutEffect(() => {
    const unsubscribe = store.subscribe(setState);
    setState(store.get());
    return unsubscribe;
  }, [store]);
  useEffect(() => refuseStrayDrops(store), []);
  useEffect(() => { setQuickOpen(false); }, [state.session?.id, state.unsent]);
  useEffect(() => {
    const handler = windowHandler();
    if (!handler) return;
    handler.postMessage({type: 'shortcuts', items: shortcuts.filter(s => s.menu).map(s => ({...s, binding: binding(s.keys[0])}))});
  }, []);
  const shortcutActions: ShortcutActions = {focusComposer, showHelp: () => setShortcutsOpen(true), showSettings: () => setSettingsOpen(true), openQuickOpen: () => setQuickOpen(true), openSessionSearch: () => setSessionSearch(true)};
  useEffect(() => {
    const run = (e: CustomEvent<{id: string}>) => nativeShortcut(e.detail.id, store, shortcutActions);
    window.addEventListener('hypercube-shortcut', run);
    return () => window.removeEventListener('hypercube-shortcut', run);
  });
  const context = useMemo<[State, Store]>(() => [state, store], [state]);

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
  const key = (e: KeyboardEvent) => {
    if (!matches(e, 'interrupt') || e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || !(e.target instanceof Element)) return;
    // Escape belongs to the terminal, dialogs, and menus while they have focus.
    if (state.confirmation || e.target.closest('[role=menu], [role=dialog], [role=alertdialog], .terminal-panel')) return;
    const session = currentSession(state);
    if (!session?.running || session.ended || session.activity !== 'busy' || state.unsent) return;
    e.preventDefault();
    post(store, 'session', {action: 'interrupt', id: session.id});
  };
  return (
    <StoreContext.Provider value={context}>
      <div className="app flex h-full flex-col" style={titleBarVars(state.titleBar)} onKeyDownCapture={e => shortcut(e, store, shortcutActions)} onKeyDown={key}>
        <div className="app-body min-h-0 flex-1">
          <Split fixed="start" size={260} minStart="160px" minEnd="50%" collapsed={sidebar ? null : 'start'}>
            <Sidebar searchOpen={sessionSearch} onSearchOpenChange={setSessionSearch} onSettings={() => setSettingsOpen(true)} />
            <Split fixed="end" size={320} minStart="60%" collapsed={rightPanel ? null : 'end'}>
              <Split split="horizontal" fixed="end" size={240} minStart="20%" minEnd="40px" collapsed={panel ? null : 'end'}>
                <main className={`app-center flex h-full min-w-0 flex-col ${tabBackground}`}><SessionView key={state.session?.id ?? (state.unsent ? 'unsent' : '')} /></main>
                <TerminalPanel key={state.session?.id ?? ''} />
              </Split>
              <RightPanel />
            </Split>
          </Split>
        </div>
        <NotificationStack />
        <ConfirmDialog />
        <QuickOpen key={state.session?.id} open={quickOpen && !state.unsent} onOpenChange={setQuickOpen} />
        <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
        <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
      </div>
    </StoreContext.Provider>
  );
}
