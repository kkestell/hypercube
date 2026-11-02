// The Workspaces view: each workspace's sessions under a header that collapses
// them, five at a time, with the workspace and session menus, and Add
// Workspace pinned below the list. A parent's caret collapses its nested
// sessions, and its Show stopped sessions row reveals the stopped ones. The
// view holds still while a session is renamed, so the field keeps its place and
// focus.
import {Fragment} from 'preact';
import {useRef, useState} from 'preact/hooks';
import {addWorkspace} from '../sessions/session-actions.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import {useStore} from '../store.js';
import {NestedSessions} from './nested-sessions.jsx';
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
  const [revealed, setRevealed] = useState({});
  const [limits, setLimits] = useState({});
  const [renaming, setRenaming] = useState(null);
  const frozen = useRef(state.workspaces);
  const menus = useSidebarMenu(store);

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);
  const toggle = id => setCollapsed(c => ({...c, [id]: !c[id]}));

  const contextMenu = e => {
    const session = sessionById(e.target.closest('[data-session]')?.dataset.session);
    const workspace = workspaces.find(w => w.id === e.target.closest('[data-workspace]')?.dataset.workspace);
    if (!session && !workspace) return;
    e.preventDefault();
    const point = {x: e.clientX, y: e.clientY};
    if (session) menus.showSession(point, session);
    else menus.showWorkspace(point, workspace);
  };

  const sessionRow = session => (
    <SessionItem
      key={session.id} session={session} selected={session.id === state.selected}
      open={!collapsed[session.id]} onToggle={session.children.length ? () => toggle(session.id) : null}
      onMenu={menus.hasSessionMenu(session) ? button => menus.showSession(below(button), session) : null}
      renaming={session.id === renaming} onRenaming={on => setRenaming(on ? session.id : null)}
    />
  );

  return (
    <div class="sidebar">
      <div class="sidebar-list" onContextMenu={contextMenu}>
        {workspaces.map(workspace => {
          const open = !collapsed[workspace.id];
          const limit = limits[workspace.id] ?? pageSize;
          return (
            <div key={workspace.id} class="sidebar-workspace" data-workspace={workspace.id}>
              <div class="sidebar-workspace-header">
                <button class="sidebar-workspace-toggle" aria-expanded={open} title={workspace.path} onClick={() => toggle(workspace.id)}>
                  <span class="sidebar-workspace-name">{workspace.name}</span>
                  <vscode-icon class="sidebar-caret" size="14" name={open ? 'chevron-down' : 'chevron-right'} />
                </button>
                <button class="sidebar-icon-button sidebar-workspace-menu" title="More Actions" aria-label="More Actions" onClick={e => menus.showWorkspace(below(e.currentTarget), workspace)}>
                  <vscode-icon name="ellipsis" />
                </button>
                <button class="sidebar-icon-button" title="New Session" aria-label="New Session" onClick={e => menus.showLeads(below(e.currentTarget), workspace)}>
                  <vscode-icon name="add" />
                </button>
              </div>
              {open && workspace.sessions.slice(0, limit).map(session => (
                <Fragment key={session.id}>
                  {sessionRow(session)}
                  <NestedSessions
                    parent={session} selected={state.selected} open={!collapsed[session.id]} revealed={revealed[session.id]} row={sessionRow}
                    onExpand={() => setCollapsed(c => ({...c, [session.id]: false}))}
                    onReveal={() => setRevealed(r => ({...r, [session.id]: true}))}
                  />
                </Fragment>
              ))}
              {open && workspace.sessions.length > limit && (
                <button class="sidebar-show-more" onClick={() => setLimits(l => ({...l, [workspace.id]: limit + pageSize}))}>Show more</button>
              )}
            </div>
          );
        })}
      </div>
      <button class="sidebar-add-workspace" onClick={() => addWorkspace(store)}>
        <vscode-icon name="add" />
        Add Workspace
      </button>
      {menus.menu && <ContextMenu {...menus.menu} onClose={menus.close} />}
    </div>
  );
}
