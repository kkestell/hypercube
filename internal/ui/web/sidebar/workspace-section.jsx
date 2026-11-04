// A workspace's section of the Sessions view, as a VS Code view pane: a header
// that collapses the section and shows its actions on hover or focus, over a
// tree of its sessions. The actions also stay shown while their menu is open,
// so Escape can return the focus to More Actions. Running sessions always
// show. Stopped ones show limit at a time once revealed, and the selected
// session's family stays visible.
import {useState} from 'preact/hooks';
import {openUnsent} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {SessionTree} from './session-tree.jsx';
import './workspace-section.css';

export const pageSize = 5;

export function WorkspaceSection({workspace, limit, onLimit, onToggleStopped, onMenu, menuOpen, showStopped, renaming, onRenaming}) {
  const [state, store] = useStore();
  const [open, setOpen] = useState(true);
  const stopped = workspace.sessions.filter(s => !s.running).slice(0, limit);
  const sessions = workspace.sessions.filter(s => s.running || stopped.includes(s)
    || s.id === state.selected || s.children.some(c => c.id === state.selected));
  const nestedOf = session => session.children.filter(s => s.running || showStopped[session.id] || s.id === state.selected);
  const stoppedLabel = limit ? 'Hide Stopped Sessions' : 'Show Stopped Sessions';
  const headingId = `workspace-section-${workspace.id}`;

  // VS Code's pane headers also collapse on Left and expand on Right.
  const headerKey = e => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault();
    setOpen(e.key === 'ArrowRight');
  };

  return (
    <section class="workspace-section" data-workspace={workspace.id} aria-labelledby={headingId}>
      <div class={`workspace-section-header${menuOpen ? ' workspace-section-header-menu-open' : ''}`} title={workspace.path}>
        <h3 class="workspace-section-heading">
          <button id={headingId} class="workspace-section-toggle" aria-expanded={open} onClick={() => setOpen(o => !o)} onKeyDown={headerKey}>
            <vscode-icon name={open ? 'chevron-down' : 'chevron-right'} />
            <span class="workspace-section-name">{workspace.name}</span>
          </button>
        </h3>
        {open && (
          <vscode-toolbar-container class="workspace-section-actions">
            <vscode-toolbar-button icon="add" label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s, workspace.id))} />
            <vscode-toolbar-button icon="history" toggleable checked={limit > 0} label={stoppedLabel} title={stoppedLabel} onClick={onToggleStopped} />
            <vscode-toolbar-button icon="ellipsis" label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)} />
          </vscode-toolbar-container>
        )}
      </div>
      {open && <SessionTree label={workspace.name} sessions={sessions} nestedOf={nestedOf} renaming={renaming} onRenaming={onRenaming} />}
      {open && limit > 0 && sessions.length < workspace.sessions.length && (
        <button class="workspace-section-more" onClick={() => onLimit(limit + pageSize)}>Show more</button>
      )}
    </section>
  );
}
