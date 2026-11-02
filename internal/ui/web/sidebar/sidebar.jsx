// The Workspaces view: each workspace's sessions under a row that collapses
// them, five at a time, with the workspace and session menus. The view holds
// still while a session is renamed, so the field keeps its place and focus.
import {useRef, useState} from 'preact/hooks';
import {addWorkspace, chooseSession} from '../sessions/session-actions.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import {useTreeOpen} from '../shared/use-tree-open.js';
import {useStore} from '../store.js';
import {SessionItem} from './session-item.jsx';
import {useSidebarMenu} from './sidebar-menus.js';
import './sidebar.css';

const pageSize = 5;

const below = button => {
  const rect = button.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

export function Sidebar() {
  const [state, store] = useStore();
  const [collapsed, setCollapsed] = useState({});
  const [limits, setLimits] = useState({});
  const [renaming, setRenaming] = useState(null);
  const frozen = useRef(state.workspaces);
  const menus = useSidebarMenu(store);
  const tree = useRef(null);

  useTreeOpen(tree, (id, open) => setCollapsed(c => ({...c, [id]: !open})));

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions).find(s => s.id === id);

  const selected = e => {
    const item = e.detail[0];
    if (item?.dataset.session) chooseSession(store, item.dataset.session);
    else if (item?.dataset.showMore) {
      const id = item.dataset.showMore;
      setLimits(l => ({...l, [id]: (l[id] ?? pageSize) + pageSize}));
    }
  };

  const contextMenu = e => {
    const item = e.target.closest('vscode-tree-item');
    const session = sessionById(item?.dataset.session);
    const workspace = workspaces.find(w => w.id === item?.dataset.key);
    if (!session && !workspace) return;
    e.preventDefault();
    const point = {x: e.clientX, y: e.clientY};
    if (session) menus.showSession(point, session);
    else menus.showWorkspace(point, workspace);
  };

  // A row's buttons keep their clicks from the row, which would toggle it.
  const action = run => e => {
    e.stopPropagation();
    run(below(e.currentTarget));
  };

  return (
    <div class="sidebar">
      <div class="sidebar-title">
        <h2 class="sidebar-title-label">Workspaces</h2>
        <vscode-toolbar-button icon="add" label="New Workspace" title="New Workspace" onClick={() => addWorkspace(store)} />
      </div>
      <vscode-tree ref={tree} class="sidebar-tree" onvsc-tree-select={selected} onContextMenu={contextMenu}>
        {workspaces.map(workspace => {
          const limit = limits[workspace.id] ?? pageSize;
          return (
            <vscode-tree-item key={workspace.id} data-key={workspace.id} branch open={!collapsed[workspace.id]} title={workspace.path}>
              <vscode-icon slot="icon-branch" name="folder" />
              <vscode-icon slot="icon-branch-opened" name="folder-opened" />
              {workspace.name}
              <vscode-toolbar-button
                slot="actions" icon="ellipsis" label="More Actions" title="More Actions"
                onClick={action(point => menus.showWorkspace(point, workspace))}
              />
              <vscode-toolbar-button
                slot="actions" icon="new-session" label="New Session" title="New Session"
                onClick={action(point => menus.showLeads(point, workspace))}
              />
              {workspace.sessions.slice(0, limit).map(session => (
                <SessionItem
                  key={session.id}
                  session={session}
                  selected={session.id === state.selected}
                  renaming={session.id === renaming}
                  onRenaming={on => setRenaming(on ? session.id : null)}
                />
              ))}
              {workspace.sessions.length > limit && (
                <vscode-tree-item class="sidebar-show-more" data-show-more={workspace.id}>Show more</vscode-tree-item>
              )}
            </vscode-tree-item>
          );
        })}
      </vscode-tree>
      {menus.menu && <ContextMenu {...menus.menu} onClose={menus.close} />}
    </div>
  );
}
