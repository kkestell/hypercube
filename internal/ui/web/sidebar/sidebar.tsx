// Workspace groups show either active sessions or archived families.
import {useCallback, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode} from 'react';
import {addWorkspace, chooseSession} from '../sessions/session-actions.ts';
import {scrollSidebar, selectSidebarTab} from '../app/workbench.ts';
import type {SidebarTab} from '../app/types.ts';
import type {Session, Workspace} from '../sessions/types.ts';
import {ContextMenu, type ContextMenuItem} from '../shared/context-menu.tsx';
import {PanelTabs} from '../shared/panel-tabs.tsx';
import {useStore} from '../store.ts';
import {sessionItems, workspaceItems} from './sidebar-menus.ts';
import {WorkspaceSection} from './workspace-section.tsx';
import {SessionSearch} from './session-search.tsx';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';

const below = (button: HTMLElement) => {
  const rect = button.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

interface Menu {
  x: number;
  y: number;
  items: ContextMenuItem[][];
}

interface SidebarProps {
  searchOpen: boolean;
  onSearchOpenChange: (open: boolean) => void;
  onSettings: () => void;
}

export function Sidebar({searchOpen, onSearchOpenChange, onSettings}: SidebarProps) {
  const [state, store] = useStore();
  const [renaming, setRenaming] = useState<string | null>(null);
  const [menu, setMenu] = useState<Menu | null>(null);
  const closeMenu = useCallback(() => setMenu(null), []);
  const view = state.workbench.sidebarTab;
  const frozen = useRef(state.workspaces);

  const visible = (workspace: Workspace) => workspace.sessions.filter(top => top.ended === (view === 'archived'));
  if (!renaming) frozen.current = state.workspaces.map(workspace => ({...workspace, sessions: visible(workspace)}));
  const workspaces = frozen.current;
  const sessionById = (id: string | undefined) => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);

  const workspaceMenu = (workspace: Workspace) => workspaceItems(store, workspace);
  const sessionMenu = (session: Session) => sessionItems(store, session, () => setRenaming(session.id));

  const contextMenu = (e: MouseEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element)) return;
    const session = sessionById(e.target.closest<HTMLElement>('[data-session]')?.dataset.session);
    const workspaceId = e.target.closest<HTMLElement>('[data-workspace]')?.dataset.workspace;
    const workspace = workspaces.find(w => w.id === workspaceId);
    const items = session ? sessionMenu(session) : workspace && workspaceMenu(workspace);
    if (!items) return;
    e.preventDefault();
    setMenu({x: e.clientX, y: e.clientY, items});
  };

  // On a focused row, F2 renames its session, and Shift+F10 or the menu key
  // opens its menu below the row.
  const keyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element)) return;
    const row = e.target.closest<HTMLElement>('[data-session]');
    const session = sessionById(row?.dataset.session);
    const menuKey = e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey);
    if (!row || !session || (e.key !== 'F2' && !menuKey)) return;
    e.preventDefault();
    if (e.key === 'F2') {
      if (session.parent || !session.ended) setRenaming(session.id);
    } else setMenu({...below(row), items: sessionMenu(session)});
  };

  const tree = <>
    {workspaces.map(workspace => (
      <WorkspaceSection
        key={workspace.id} workspace={workspace}
        showDraft={view === 'sessions'}
        onMenu={button => setMenu({...below(button), items: workspaceMenu(workspace)})}
        renaming={renaming} onRenaming={setRenaming}
      />
    ))}
  </>;

  const bodies = (['sessions', 'archived'] as const).map(tab => <SidebarBody key={tab} tab={tab} active={view === tab} onContextMenu={contextMenu} onKeyDown={keyDown}>
    {view === tab && tree}
  </SidebarBody>);

  return (
    <div className="sidebar flex h-full min-h-0 flex-col bg-card">
      <PanelTabs label="Sidebar" tabs={[{id: 'sessions', title: 'Sessions', icon: 'comment-discussion'}, {id: 'archived', title: 'Archived', icon: 'archive'}]}
        selected={view} onSelect={value => { setRenaming(null); store.update(s => selectSidebarTab(s, value)); }} clearsWindowButtons actions={<>
          <SessionSearch open={searchOpen} onOpenChange={onSearchOpenChange} onSelect={id => chooseSession(store, id)} />
          <Button className="size-7" variant="ghost" size="icon" aria-label="Settings" title="Settings" onClick={onSettings}><Icon name="settings" /></Button>
          <Button className="size-7" variant="ghost" size="icon" aria-label="Add Workspace" title="Add Workspace" onClick={() => addWorkspace(store)}><Icon name="new-folder" /></Button>
        </>}>
        {bodies}
      </PanelTabs>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={closeMenu} />}
    </div>
  );
}

// Each tab owns its native scroller even while its rows are hidden.
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
  shown.current = active;
  useLayoutEffect(() => {
    if (!active || !ref.current) return;
    top.current = store.get().workbench.sidebarScroll[tab];
    ref.current.scrollTop = top.current;
  }, [active, state.workspaces]);
  useLayoutEffect(() => {
    const save = () => store.update(s => scrollSidebar(s, tab, top.current));
    const pagehide = () => {
      if (shown.current && ref.current) top.current = ref.current.scrollTop;
      save();
    };
    window.addEventListener('pagehide', pagehide);
    return () => {
      save();
      window.removeEventListener('pagehide', pagehide);
    };
  }, [store, tab]);
  return <div ref={ref} className="sidebar-body h-full min-h-0 overflow-auto p-2 pt-1" data-sidebar-tab={tab}
    onContextMenu={onContextMenu} onKeyDown={onKeyDown} onScroll={event => {
      if (!active) return;
      top.current = event.currentTarget.scrollTop;
      store.update(s => scrollSidebar(s, tab, top.current));
    }}>{children}</div>;
}
