// The Sessions view: a collapsible section per
// workspace, each holding a tree of its sessions. The list freezes during
// renaming so the field keeps its place and focus.
import {useCallback, useRef, useState} from 'react';
import {addWorkspace} from '../sessions/session-actions.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import {PanelTabs} from '../shared/panel-tabs.jsx';
import {useStore} from '../store.js';
import {sessionItems, workspaceItems} from './sidebar-menus.js';
import {WorkspaceSection} from './workspace-section.jsx';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';

const below = button => {
  const rect = button.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

export function Sidebar() {
  const [state, store] = useStore();
  const [renaming, setRenaming] = useState(null);
  // menu.workspace names the section whose More Actions opened the menu.
  const [menu, setMenu] = useState(null);
  const closeMenu = useCallback(() => setMenu(null), []);
  const frozen = useRef(state.workspaces);

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);

  const workspaceMenu = workspace => workspaceItems(store, workspace);
  const sessionMenu = session => sessionItems(store, session, () => setRenaming(session.id));

  const contextMenu = e => {
    const session = sessionById(e.target.closest('[data-session]')?.dataset.session);
    const workspace = workspaces.find(w => w.id === e.target.closest('[data-workspace]')?.dataset.workspace);
    if (!session && !workspace) return;
    e.preventDefault();
    setMenu({x: e.clientX, y: e.clientY, items: session ? sessionMenu(session) : workspaceMenu(workspace)});
  };

  // On a focused row, F2 renames its session, and Shift+F10 or the menu key
  // opens its menu below the row.
  const keyDown = e => {
    const row = e.target.closest('[data-session]');
    const menuKey = e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey);
    if (!row || (e.key !== 'F2' && !menuKey)) return;
    e.preventDefault();
    if (e.key === 'F2') setRenaming(row.dataset.session);
    else setMenu({...below(row), items: sessionMenu(sessionById(row.dataset.session))});
  };

  return (
    <div className="sidebar flex h-full min-h-0 flex-col">
      <PanelTabs label="Sidebar" tabs={[{id: 'sessions', title: 'Sessions', icon: 'comment-discussion'}]} selected="sessions" actions={<>
          <Button variant="ghost" size="icon" aria-label="Add Workspace" title="Add Workspace" onClick={() => addWorkspace(store)}><Icon name="new-folder" /></Button>
        </>}>
        <div className="sidebar-body min-h-0 flex-1 overflow-auto p-2" onContextMenu={contextMenu} onKeyDown={keyDown}>
          {workspaces.map(workspace => (
            <WorkspaceSection
              key={workspace.id} workspace={workspace}
              menuOpen={menu?.workspace === workspace.id}
              onMenu={button => setMenu({...below(button), items: workspaceMenu(workspace), workspace: workspace.id})}
              renaming={renaming} onRenaming={setRenaming}
            />
          ))}
        </div>
      </PanelTabs>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={closeMenu} />}
    </div>
  );
}
