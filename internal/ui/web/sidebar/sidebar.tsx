// Workspace groups show either active sessions or archived families.
import {useCallback, useEffect, useRef, useState, type KeyboardEvent, type MouseEvent} from 'react';
import {addWorkspace, chooseSession} from '../sessions/session-actions.ts';
import {sessionOf} from '../sessions/selection.ts';
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
  const [view, setView] = useState('sessions');
  const frozen = useRef(state.workspaces);
  const family = sessionOf(state, state.selected);
  const selectedView = family ? ((family.parent ?? family.session).ended ? 'archived' : 'sessions') : null;
  useEffect(() => {
    if (selectedView) setView(selectedView);
  }, [state.selected, selectedView]);

  const selectSearchResult = (id: string) => {
    const family = sessionOf(state, id);
    if (family) setView((family.parent ?? family.session).ended ? 'archived' : 'sessions');
    chooseSession(store, id);
  };

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

  const tree = <div className="sidebar-body min-h-0 overflow-auto p-2 pt-1" onContextMenu={contextMenu} onKeyDown={keyDown}>
    {workspaces.map(workspace => (
      <WorkspaceSection
        key={workspace.id} workspace={workspace}
        onMenu={button => setMenu({...below(button), items: workspaceMenu(workspace)})}
        renaming={renaming} onRenaming={setRenaming}
      />
    ))}
  </div>;

  return (
    <div className="sidebar flex h-full min-h-0 flex-col bg-card">
      <PanelTabs label="Sidebar" tabs={[{id: 'sessions', title: 'Sessions', icon: 'comment-discussion'}, {id: 'archived', title: 'Archived', icon: 'archive'}]}
        selected={view} onSelect={value => { setRenaming(null); setView(value); }} clearsWindowButtons actions={<>
          <SessionSearch open={searchOpen} onOpenChange={onSearchOpenChange} onSelect={selectSearchResult} />
          <Button className="size-7" variant="ghost" size="icon" aria-label="Settings" title="Settings" onClick={onSettings}><Icon name="settings" /></Button>
          <Button className="size-7" variant="ghost" size="icon" aria-label="Add Workspace" title="Add Workspace" onClick={() => addWorkspace(store)}><Icon name="new-folder" /></Button>
        </>}>
        <>{view === 'sessions' && tree}</>
        <>{view === 'archived' && tree}</>
      </PanelTabs>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={closeMenu} />}
    </div>
  );
}
