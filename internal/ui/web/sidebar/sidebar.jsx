// The Sessions view, laid out as VS Code's Explorer: a collapsible section per
// workspace, each holding a tree of its sessions. The list freezes during
// renaming so the field keeps its place and focus.
import {useCallback, useRef, useState} from 'preact/hooks';
import {addWorkspace} from '../sessions/session-actions.js';
import {openUnsent} from '../sessions/selection.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import '../shared/panel-tabs.css';
import {useStore} from '../store.js';
import {sessionItems, workspaceItems} from './sidebar-menus.js';
import {pageSize, WorkspaceSection} from './workspace-section.jsx';
import './sidebar.css';

const below = button => {
  const rect = button.getBoundingClientRect();
  return {x: rect.left, y: rect.bottom};
};

export function Sidebar() {
  const [state, store] = useStore();
  // showStopped is per parent session; stoppedLimits is per workspace, and 0
  // hides the workspace's stopped sessions.
  const [showStopped, setShowStopped] = useState({});
  const [stoppedLimits, setStoppedLimits] = useState({});
  const [renaming, setRenaming] = useState(null);
  const [menu, setMenu] = useState(null);
  const closeMenu = useCallback(() => setMenu(null), []);
  const frozen = useRef(state.workspaces);

  if (!renaming) frozen.current = state.workspaces;
  const workspaces = frozen.current;
  const sessionById = id => workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children])).find(s => s.id === id);

  const setLimit = (workspace, limit) => setStoppedLimits(l => ({...l, [workspace.id]: limit}));
  const toggleStopped = workspace => setLimit(workspace, stoppedLimits[workspace.id] ? 0 : pageSize);
  const workspaceMenu = workspace => workspaceItems(store, workspace, !!stoppedLimits[workspace.id], () => toggleStopped(workspace));
  const sessionMenu = session => sessionItems(store, session, {
    stoppedShown: showStopped[session.id],
    toggleStopped: () => setShowStopped(s => ({...s, [session.id]: !s[session.id]})),
    rename: () => setRenaming(session.id),
  });

  const contextMenu = e => {
    const session = sessionById(e.target.closest('[data-session]')?.dataset.session);
    const workspace = workspaces.find(w => w.id === e.target.closest('[data-workspace]')?.dataset.workspace);
    if (!session && !workspace) return;
    e.preventDefault();
    setMenu({x: e.clientX, y: e.clientY, items: session ? sessionMenu(session) : workspaceMenu(workspace)});
  };

  // F2 renames the focused session, as in VS Code's Explorer.
  const keyDown = e => {
    const id = e.key === 'F2' && e.target.closest('[data-session]')?.dataset.session;
    if (!id) return;
    e.preventDefault();
    setRenaming(id);
  };

  return (
    <div class="sidebar">
      <vscode-tabs class="panel-tabs" panel selected-index="0">
        <vscode-tab-header slot="header" title="Sessions" aria-label="Sessions">
          <vscode-icon name="comment-discussion" />
        </vscode-tab-header>
        <vscode-toolbar-container slot="addons">
          <vscode-toolbar-button icon="add" label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s))} />
          <vscode-toolbar-button icon="new-folder" label="Add Workspace" title="Add Workspace" onClick={() => addWorkspace(store)} />
        </vscode-toolbar-container>
        <vscode-tab-panel class="sidebar-body" onContextMenu={contextMenu} onKeyDown={keyDown}>
          {workspaces.map(workspace => (
            <WorkspaceSection
              key={workspace.id} workspace={workspace}
              limit={stoppedLimits[workspace.id] ?? 0} onLimit={limit => setLimit(workspace, limit)}
              onToggleStopped={() => toggleStopped(workspace)}
              onMenu={button => setMenu({...below(button), items: workspaceMenu(workspace)})}
              showStopped={showStopped} renaming={renaming} onRenaming={setRenaming}
            />
          ))}
        </vscode-tab-panel>
      </vscode-tabs>
      {menu && <ContextMenu {...menu} onClose={closeMenu} />}
    </div>
  );
}
