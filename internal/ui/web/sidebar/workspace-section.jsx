// A workspace's section of the Sessions view, as a VS Code view pane: a header
// that collapses the section and shows its actions on hover or focus, over a
// tree of its sessions. The actions also stay shown while their menu is open,
// so Escape can return the focus to More Actions.
import {useState} from 'preact/hooks';
import {openUnsent} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {SessionTree} from './session-tree.jsx';
import './workspace-section.css';

export function WorkspaceSection({workspace, onMenu, menuOpen, renaming, onRenaming}) {
  const [, store] = useStore();
  const [open, setOpen] = useState(true);
  const headingId = `workspace-section-${workspace.id}`;

  // VS Code's pane headers also collapse on Left and expand on Right.
  const headerKey = e => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault();
    setOpen(e.key === 'ArrowRight');
  };

  return (
    <section class="workspace-section" data-workspace={workspace.id} aria-labelledby={headingId}>
      <div class={`workspace-section-header panel-row${menuOpen ? ' workspace-section-header-menu-open' : ''}`} title={workspace.path}>
        <h3 class="workspace-section-heading">
          <button id={headingId} class="workspace-section-toggle" aria-expanded={open} onClick={() => setOpen(o => !o)} onKeyDown={headerKey}>
            <vscode-icon class="panel-row-icon" name={open ? 'chevron-down' : 'chevron-right'} />
            <span class="workspace-section-name">{workspace.name}</span>
          </button>
        </h3>
        {open && (
          <vscode-toolbar-container class="workspace-section-actions">
            <vscode-toolbar-button icon="add" label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s, workspace.id))} />
            <vscode-toolbar-button icon="ellipsis" label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)} />
          </vscode-toolbar-container>
        )}
      </div>
      {open && <SessionTree label={workspace.name} sessions={workspace.sessions} renaming={renaming} onRenaming={onRenaming} />}

    </section>
  );
}
