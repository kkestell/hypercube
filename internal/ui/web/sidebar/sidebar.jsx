// Sessions are grouped by workspace, with children beneath their parent.
// Workspace actions reveal stopped parents
// five at a time; the selected family stays visible. Freeze the list during
// renaming so the field keeps its place and focus.
import {useCallback, useRef, useState} from 'preact/hooks';
import {addWorkspace} from '../sessions/session-actions.js';
import {openUnsent} from '../sessions/selection.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import '../shared/panel-tabs.css';
import {useStore} from '../store.js';
import {SessionItem} from './session-item.jsx';
import {sessionItems, workspaceItems} from './sidebar-menus.js';
import './sidebar.css';

const pageSize = 5;

const below = button => {
  const rect = button.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

export function Sidebar() {
  const [state, store] = useStore();
  const [showStopped, setShowStopped] = useState({});
  const [collapsed, setCollapsed] = useState({});
  const [stoppedLimits, setStoppedLimits] = useState({});
  const [renaming, setRenaming] = useState(null);
  const [menu, setMenu] = useState(null);
  const closeMenu = useCallback(() => setMenu(null), []);
  const frozen = useRef(state.workspaces);
  const showMenu = (point, items) => {
    if (items.length) setMenu({...point, items});
  };
  const itemsOf = session => sessionItems(store, session, showStopped[session.id], () => setShowStopped(s => ({...s, [session.id]: !s[session.id]})));
  const workspaceItemsOf = workspace => workspaceItems(store, workspace, !!stoppedLimits[workspace.id],
    () => setStoppedLimits(l => ({...l, [workspace.id]: l[workspace.id] ? 0 : pageSize})));

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);

  const contextMenu = e => {
    const session = sessionById(e.target.closest('[data-session]')?.dataset.session);
    const workspace = workspaces.find(w => w.id === e.target.closest('[data-workspace]')?.dataset.workspace);
    if (!session && !workspace) return;
    e.preventDefault();
    const point = {x: e.clientX, y: e.clientY};
    showMenu(point, session ? itemsOf(session) : workspaceItemsOf(workspace));
  };

  const workspaceMenu = button => {
    const point = below(button);
    showMenu(point, [
      ...workspaces.map(workspace => ({label: workspace.name, run: () => showMenu(point, workspaceItemsOf(workspace))})),
      {label: 'Add Workspace', run: () => addWorkspace(store)},
    ]);
  };

  const toggle = id => setCollapsed(c => ({...c, [id]: !c[id]}));
  const sessionRow = (session, hasChildren = false) => {
    return (
      <SessionItem
        key={session.id} session={session} selected={session.id === state.selected}
        hasChildren={hasChildren} expanded={!collapsed[`session:${session.id}`]} onToggle={() => toggle(`session:${session.id}`)}
        renaming={session.id === renaming} onRenaming={on => setRenaming(on ? session.id : null)}
      />
    );
  };

  return (
    <div class="sidebar">
      <vscode-tabs class="panel-tabs" panel selected-index="0">
        <vscode-tab-header slot="header" title="Sessions" aria-label="Sessions">
          <vscode-icon name="comment-discussion" />
        </vscode-tab-header>
        <div slot="addons" class="sidebar-actions">
          <button class="sidebar-icon-button" title="Workspace Actions" aria-label="Workspace Actions" onClick={e => workspaceMenu(e.currentTarget)}>
            <vscode-icon name="ellipsis" />
          </button>
        </div>
        <vscode-tab-panel class="sidebar-body">
          <div class="sidebar-list" onContextMenu={contextMenu}>
            {workspaces.map(workspace => {
              const limit = stoppedLimits[workspace.id] ?? 0;
              const stopped = workspace.sessions.filter(s => !s.running).slice(0, limit);
              const sessions = workspace.sessions.filter(s => s.running || stopped.includes(s)
                || s.id === state.selected || s.children.some(c => c.id === state.selected));
              return (
                <div key={workspace.id} data-workspace={workspace.id}>
                  <button class="sidebar-workspace" title={workspace.path} aria-expanded={!collapsed[`workspace:${workspace.id}`]} onClick={() => toggle(`workspace:${workspace.id}`)}>
                    <vscode-icon name={collapsed[`workspace:${workspace.id}`] ? 'chevron-right' : 'chevron-down'} />
                    <span>{workspace.name}</span>
                  </button>
                  {!collapsed[`workspace:${workspace.id}`] && sessions.map(session => {
                    // A parent's running children, the selected one, and
                    // stopped children when the parent's menu reveals them.
                    const nested = session.children.filter(s => s.running || showStopped[session.id] || s.id === state.selected);
                    return (
                      <div key={session.id} class="sidebar-session-group" data-workspace={workspace.id}>
                        {sessionRow(session, nested.length > 0)}
                        {nested.length > 0 && !collapsed[`session:${session.id}`] && <div class="sidebar-nested">{nested.map(child => sessionRow(child))}</div>}
                      </div>
                    );
                  })}
                  {!collapsed[`workspace:${workspace.id}`] && limit > 0 && sessions.length < workspace.sessions.length && (
                    <button class="sidebar-show-more" data-workspace={workspace.id} onClick={() => setStoppedLimits(l => ({...l, [workspace.id]: limit + pageSize}))}>Show more in {workspace.name}</button>
                  )}
                </div>
              );
            })}
          </div>
          <button class="sidebar-new-session" title="New Session" onClick={() => store.update(s => openUnsent(s))}>
            <vscode-icon name="add" />
            New Session
          </button>
        </vscode-tab-panel>
      </vscode-tabs>
      {menu && <ContextMenu {...menu} onClose={closeMenu} />}
    </div>
  );
}
