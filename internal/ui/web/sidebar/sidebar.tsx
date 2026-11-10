// Workspace groups show either active sessions or archived families.
import {useCallback, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode} from 'react';
import {addWorkspace} from '../sessions/session-actions.ts';
import {homeShown, showHome} from '../sessions/selection.ts';
import {isUnsent} from '../agent/drafts.ts';
import {hypercube} from '../app/window.ts';
import {openSettings, scrollSidebar, selectSidebarTab} from '../app/workbench.ts';
import type {SidebarTab} from '../app/types.ts';
import type {Session} from '../sessions/types.ts';
import {ContextMenu, type ContextMenuItem} from '../shared/context-menu.tsx';
import {IconButton} from '../shared/icon-button.tsx';
import {PanelTabs} from '../shared/panel-tabs.tsx';
import {useStore} from '../store.ts';
import {sessionItems, workspaceItems} from './sidebar-menus.ts';
import {sidebarSession, sidebarWorkspaces, startRenaming, stopRenaming} from './sidebar-rows.ts';
import {WorkspaceSection} from './workspace-section.tsx';
import {SessionSearch} from './session-search.tsx';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
import {yourMoves} from '../github/turns.ts';

const below = (element: HTMLElement) => {
  const rect = element.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

interface Menu {
  x: number;
  y: number;
  items: ContextMenuItem[][];
}

export function Sidebar() {
  const [state, store] = useStore();
  // The last menu stays drawn while it closes.
  const [menu, setMenu] = useState<Menu | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const showMenu = (next: Menu) => { setMenu(next); setMenuOpen(true); };
  const closeMenu = useCallback(() => setMenuOpen(false), []);
  const view = state.workbench.sidebarTab;
  const workspaces = sidebarWorkspaces(state);
  const sessionMenu = (session: Session) => sessionItems(store, session, () => store.update(s => startRenaming(s, session.id)));
  const workspaceOf = (target: Element) => state.workspaces.find(w => w.id === target.closest<HTMLElement>('[data-workspace]')?.dataset.workspace);

  const contextMenu = (e: MouseEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element)) return;
    const session = sidebarSession(state, e.target.closest<HTMLElement>('[data-session]')?.dataset.session);
    const workspace = workspaceOf(e.target);
    const items = session ? sessionMenu(session) : workspace && workspaceItems(store, workspace);
    if (!items) return;
    e.preventDefault();
    showMenu({x: e.clientX, y: e.clientY, items});
  };

  // On a focused row, F2 renames its session, and Shift+F10 or the menu key
  // opens its session's or workspace's menu below the row.
  const keyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element)) return;
    const menuKey = e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey);
    const header = e.target.closest<HTMLElement>('[data-workspace-header]');
    const workspace = header && workspaceOf(header);
    if (menuKey && header && workspace) {
      e.preventDefault();
      showMenu({...below(header), items: workspaceItems(store, workspace)});
      return;
    }
    const row = e.target.closest<HTMLElement>('[data-session]');
    const session = sidebarSession(state, row?.dataset.session);
    if (!row || !session || (e.key !== 'F2' && !menuKey)) return;
    e.preventDefault();
    if (e.key === 'F2') {
      if (!isUnsent(session.id) && (session.parent || !session.ended)) store.update(s => startRenaming(s, session.id));
    } else showMenu({...below(row), items: sessionMenu(session)});
  };

  const needYou = yourMoves(state.accounts).length;
  const home = homeShown(state);
  const tree = <>
    {view === 'sessions' && workspaces.length > 0 && state.accounts.length > 0 && <div className="sticky top-0 z-10 bg-sidebar pb-2">
      <Button variant={home ? 'secondary' : 'ghost'} size="sm" className={`h-8 w-full justify-start gap-2 px-2 font-normal ${home ? 'hover:bg-secondary' : ''}`} aria-pressed={home} onClick={() => store.update(s => showHome(stopRenaming(s)))}>
        <Icon name="git-pull-request" className="text-muted-foreground" />
        <span className="flex-1 text-left">Pull Requests</span>
        {needYou > 0 && <span className="text-xs text-muted-foreground">{needYou}</span>}
      </Button>
    </div>}
    {view === 'sessions' && workspaces.length > 0 && <h2 className="mt-2 flex h-8 items-center px-2 text-xs font-medium text-muted-foreground">Workspaces</h2>}
    {workspaces.map(({workspace, groups}) => <WorkspaceSection key={workspace.id} workspace={workspace} groups={groups} />)}
  </>;

  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar" data-region="sidebar">
      <PanelTabs label="Sidebar" tabs={[{id: 'sessions', title: 'Sessions', icon: 'comment-discussion'}, {id: 'archived', title: 'Archived', icon: 'archive'}]}
        selected={view} onSelect={value => store.update(s => selectSidebarTab(stopRenaming(s), value))} clearsWindowButtons actions={<>
          <SessionSearch />
          <IconButton icon="settings" label="Settings" shortcut="settings" onClick={() => store.update(s => openSettings(s))} />
          {hypercube() && <IconButton icon="new-folder" label="Add Workspace" onClick={() => addWorkspace(store)} />}
        </>}>
        {(['sessions', 'archived'] as const).map(tab => <SidebarBody key={tab} tab={tab} active={view === tab} onContextMenu={contextMenu} onKeyDown={keyDown}>
          {view === tab && tree}
        </SidebarBody>)}
      </PanelTabs>
      {menu && <ContextMenu open={menuOpen} x={menu.x} y={menu.y} items={menu.items} onClose={closeMenu} />}
    </div>
  );
}

// Each tab owns its native scroller even while its rows are hidden. Its
// position is kept here while it scrolls and saved when the tab is hidden, the
// sidebar unmounts, or the page hides.
function SidebarBody({tab, active, onContextMenu, onKeyDown, children}: {
  tab: SidebarTab;
  active: boolean;
  onContextMenu: (event: MouseEvent<HTMLDivElement>) => void;
  onKeyDown: (event: KeyboardEvent<HTMLDivElement>) => void;
  children: ReactNode;
}) {
  const [state, store] = useStore();
  const ref = useRef<HTMLDivElement>(null);
  const top = useRef(state.workbench.sidebarScroll[tab]);
  const shown = useRef(active);
  // save records the shown tab's position, then stores it.
  const save = useCallback(() => {
    if (shown.current && ref.current) top.current = ref.current.scrollTop;
    store.update(s => scrollSidebar(s, tab, top.current));
  }, [store, tab]);
  // Rows arriving with the workspaces make room to restore the position.
  useLayoutEffect(() => {
    if (active && ref.current) ref.current.scrollTop = top.current;
  }, [active, state.workspaces]);
  useLayoutEffect(() => {
    shown.current = active;
    if (!active) store.update(s => scrollSidebar(s, tab, top.current));
  }, [active, store, tab]);
  useLayoutEffect(() => {
    window.addEventListener('pagehide', save);
    return () => {
      save();
      window.removeEventListener('pagehide', save);
    };
  }, [save]);
  return <div ref={ref} className="h-full min-h-0 overflow-auto p-2 pt-1" data-sidebar-tab={tab}
    onContextMenu={onContextMenu} onKeyDown={onKeyDown} onScroll={event => {
      if (active) top.current = event.currentTarget.scrollTop;
    }}>{children}</div>;
}
