// The Workspaces view: each workspace's started sessions under its name, with
// Show more revealing stopped ones five at a time, the workspace and session
// menus, and Add Workspace pinned below the list. Started children are always listed under their parent; its menu
// shows or hides stopped children.
// The view holds still while a session is renamed, so the field keeps its
// place and focus.
import {Fragment} from 'preact';
import {useRef, useState} from 'preact/hooks';
import {addWorkspace} from '../sessions/session-actions.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import '../shared/panel-tabs.css';
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
  const [showStopped, setShowStopped] = useState({});
  const [limits, setLimits] = useState({});
  const [renaming, setRenaming] = useState(null);
  const frozen = useRef(state.workspaces);
  const toggleStopped = id => setShowStopped(s => ({...s, [id]: !s[id]}));
  const menus = useSidebarMenu(store, showStopped, toggleStopped);

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);

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
      onMenu={menus.hasSessionMenu(session) ? button => menus.showSession(below(button), session) : null}
      renaming={session.id === renaming} onRenaming={on => setRenaming(on ? session.id : null)}
    />
  );

  return (
    <div class="sidebar">
      <vscode-tabs class="panel-tabs" panel selected-index="0">
        <vscode-tab-header slot="header" title="Sessions" aria-label="Sessions">
          <vscode-icon name="comment-discussion" />
        </vscode-tab-header>
        <vscode-tab-panel class="sidebar-body">
          <div class="sidebar-list" onContextMenu={contextMenu}>
            {workspaces.map(workspace => {
              const limit = limits[workspace.id] ?? 0;
              const stopped = workspace.sessions.filter(s => !s.running && s.id !== state.selected);
              const hidden = new Set(stopped.slice(limit).map(s => s.id));
              return (
                <div key={workspace.id} class="sidebar-workspace" data-workspace={workspace.id}>
                  <div class="sidebar-workspace-header">
                    <span class="sidebar-workspace-name" title={workspace.path}>{workspace.name}</span>
                    <button class="sidebar-icon-button sidebar-workspace-menu" title="More Actions" aria-label="More Actions" onClick={e => menus.showWorkspace(below(e.currentTarget), workspace)}>
                      <vscode-icon name="ellipsis" />
                    </button>
                    <button class="sidebar-icon-button" title="New Session" aria-label="New Session" onClick={e => menus.showLeads(below(e.currentTarget), workspace)}>
                      <vscode-icon name="add" />
                    </button>
                  </div>
                  {workspace.sessions.length === 0 && <div class="sidebar-empty">No sessions</div>}
                  {workspace.sessions.filter(s => !hidden.has(s.id)).map(session => (
                    <Fragment key={session.id}>
                      {sessionRow(session)}
                      <NestedSessions
                        parent={session} selected={state.selected}
                        showStopped={showStopped[session.id]} row={sessionRow}
                      />
                    </Fragment>
                  ))}
                  {hidden.size > 0 && (
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
        </vscode-tab-panel>
      </vscode-tabs>
      {menus.menu && <ContextMenu {...menus.menu} onClose={menus.close} />}
    </div>
  );
}
