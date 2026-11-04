// A workspace's section of the Sessions view: a header with the workspace's
// actions over a tree of its sessions, children beneath their parent. Running
// sessions always show. Stopped ones show limit at a time once revealed, and
// the selected session's family stays visible.
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import {SessionItem} from './session-item.jsx';
import './workspace-section.css';

export const pageSize = 5;

// The tree expands on double-click so that a click on a parent only selects
// it; a click on its chevron still expands or collapses it.
const toggleOnChevron = e => {
  if (!e.composedPath().some(node => node.part?.contains('arrow-icon-container'))) return;
  e.stopPropagation();
  const item = e.target.closest('vscode-tree-item');
  item.open = !item.open;
};

export function WorkspaceSection({workspace, limit, onLimit, onToggleStopped, onMenu, showStopped, renaming, onRenaming}) {
  const [state, store] = useStore();
  const stopped = workspace.sessions.filter(s => !s.running).slice(0, limit);
  const sessions = workspace.sessions.filter(s => s.running || stopped.includes(s)
    || s.id === state.selected || s.children.some(c => c.id === state.selected));
  const nestedOf = session => session.children.filter(s => s.running || showStopped[session.id] || s.id === state.selected);
  const stoppedLabel = limit ? 'Hide Stopped Sessions' : 'Show Stopped Sessions';

  const selected = e => {
    const id = e.detail[0]?.dataset.session;
    if (id) chooseSession(store, id);
  };

  return (
    <vscode-collapsible class="workspace-section" heading={workspace.name} title={workspace.path} open data-workspace={workspace.id}>
      <vscode-toolbar-button slot="actions" icon="history" toggleable checked={limit > 0} label={stoppedLabel} title={stoppedLabel} onClick={onToggleStopped} />
      <vscode-toolbar-button slot="actions" icon="ellipsis" label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)} />
      <vscode-tree class="workspace-section-tree" aria-label={workspace.name} expand-mode="doubleClick" onClickCapture={toggleOnChevron} onvsc-tree-select={selected}>
        {sessions.map(session => (
          <SessionItem key={session.id} session={session} nested={nestedOf(session)} renaming={renaming} onRenaming={onRenaming} />
        ))}
      </vscode-tree>
      {limit > 0 && sessions.length < workspace.sessions.length && (
        <button class="workspace-section-more" onClick={() => onLimit(limit + pageSize)}>Show more</button>
      )}
    </vscode-collapsible>
  );
}
