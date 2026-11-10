// The page frame: the sidebar, the center with the terminal panel below it,
// the right panel, and the status bar, with the notification cards and the
// confirmation dialog over them. App draws from the store's state,
// keeps the page title and URL hash in step with it, and refuses drops that
// would land outside their targets: a tab dragged anywhere but a pane could
// type its ID into a text box, and a file outside an agent pane would open in
// the browser. A pane or an agent pane takes a drop by preventing its default.
import {useEffect, useLayoutEffect, useState} from 'react';
import {post} from '../api.ts';
import {QuickOpen} from '../files/quick-open.tsx';
import {NotificationStack} from '../notifications/notification-stack.tsx';
import {currentSession, pageTitle} from '../sessions/selection.ts';
import {SessionView} from '../sessions/session-view.tsx';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {Split} from '../shared/split.tsx';
import {Sidebar} from '../sidebar/sidebar.tsx';
import {StoreProvider, type Store} from '../store.ts';
import {TerminalPanel} from '../terminal/terminal-panel.tsx';
import {RightPanel} from './right-panel.tsx';
import {binding} from './keys.ts';
import {keysOf, matches, nativeShortcut, shortcut, shortcuts} from './shortcuts.ts';
import {SettingsDialog} from './settings-dialog.tsx';
import {ShortcutsDialog} from './shortcuts-dialog.tsx';
import {hypercube} from './window.ts';
import {titleBarVars} from './title-bar.ts';
import {hideRegion, resizeRegion} from './workbench.ts';

function refuseStrayDrops(store: Store) {
  const refuse = (e: DragEvent) => {
    const draggingTab = !!store.get().editor?.tabs.dragging;
    const strayFile = !!e.dataTransfer?.types.includes('Files');
    if ((!draggingTab && !strayFile) || e.defaultPrevented) return;
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

// interrupt stops the shown session's agent on Escape. Escape belongs to the
// terminal, dialogs, and menus while they have focus.
function interrupt(e: KeyboardEvent, store: Store) {
  if (!matches(e, 'interrupt') || e.defaultPrevented || e.repeat || e.isComposing || !(e.target instanceof Element)) return;
  const state = store.get();
  if (state.confirmation || e.target.closest('[role=menu], [role=dialog], [role=alertdialog], [data-region=panel]')) return;
  const session = currentSession(state);
  if (!session || session.ended || session.activity !== 'busy' || state.unsent) return;
  e.preventDefault();
  post(store, 'session', {action: 'interrupt', id: session.id});
}

function listenForKeys(store: Store) {
  const capture = (e: KeyboardEvent) => shortcut(e, store);
  const key = (e: KeyboardEvent) => interrupt(e, store);
  const native = (e: CustomEvent<{id: string}>) => nativeShortcut(e.detail.id, store);
  document.addEventListener('keydown', capture, true);
  document.addEventListener('keydown', key);
  window.addEventListener('hypercube-shortcut', native);
  return () => {
    document.removeEventListener('keydown', capture, true);
    document.removeEventListener('keydown', key);
    window.removeEventListener('hypercube-shortcut', native);
  };
}

export function App({store}: {store: Store}) {
  const [state, setState] = useState(store.get());
  useLayoutEffect(() => {
    const unsubscribe = store.subscribe(setState);
    setState(store.get());
    return unsubscribe;
  }, [store]);
  useEffect(() => refuseStrayDrops(store), [store]);
  useEffect(() => listenForKeys(store), [store]);
  useEffect(() => {
    hypercube()?.postMessage({type: 'shortcuts', items: shortcuts.filter(s => s.menu).map(s => ({id: s.id, section: s.section, title: s.title, bindings: keysOf(s).map(binding)}))});
  }, []);

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

  const {sidebar, rightPanel, sizes} = state.workbench;
  const panel = state.editor?.panel ?? false;
  return (
    <StoreProvider state={state} store={store}>
      <div className="flex h-full flex-col" style={titleBarVars(state.titleBar)}>
        <div className="min-h-0 flex-1">
          <Split fixed="start" size={sizes.sidebar} onResize={size => store.update(s => resizeRegion(s, 'sidebar', size))} onCollapse={() => store.update(s => hideRegion(s, 'sidebar'))} minStart="280px" minEnd="50%" collapsed={sidebar ? null : 'start'}>
            <Sidebar />
            <Split fixed="end" size={sizes.rightPanel} onResize={size => store.update(s => resizeRegion(s, 'rightPanel', size))} onCollapse={() => store.update(s => hideRegion(s, 'rightPanel'))} minEnd="280px" collapsed={rightPanel ? null : 'end'}>
              <Split split="horizontal" fixed="end" size={sizes.panel} onResize={size => store.update(s => resizeRegion(s, 'panel', size))} onCollapse={() => store.update(s => hideRegion(s, 'panel'))} minStart="20%" minEnd="40px" collapsed={panel ? null : 'end'}>
                <main className="flex h-full min-w-0 flex-col bg-tab"><SessionView key={state.editor?.id ?? (state.unsent ? 'unsent' : '')} /></main>
                <TerminalPanel key={state.editor?.id ?? ''} />
              </Split>
              <RightPanel />
            </Split>
          </Split>
        </div>
        <NotificationStack />
        <ConfirmDialog />
        <QuickOpen />
        <SettingsDialog />
        <ShortcutsDialog />
      </div>
    </StoreProvider>
  );
}
