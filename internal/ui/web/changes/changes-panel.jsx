// The session's repository branch and flat list of uncommitted files.
import {useEffect} from 'preact/hooks';
import {diffTabId, openDiffTab} from '../editor/editor.js';
import {panes} from '../editor/tab-layout.js';
import {mergeSummary} from '../sessions/merge.js';
import {currentSession} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {loadChanges, statusLabel} from './changes.js';
import './changes-panel.css';

export function ChangesPanel() {
  const [state, store] = useStore();
  const session = state.session;
  useEffect(() => { loadChanges(store); }, [session?.id]);
  const changes = session?.changes;
  if (!changes) return null;
  if (changes.missing || changes.error) return <div class="changes-panel-message">{changes.missing ? 'The folder no longer exists.' : changes.error}</div>;
  const merge = currentSession(state)?.merge;
  const active = new Set(panes(session.tabs.root).map(pane => pane.active));
  return (
    <div class="changes-panel">
      <div class="changes-panel-row changes-panel-branch">
        <span class="changes-panel-name">{changes.branch}</span>
        {merge?.unmerged > 0 && <span class="changes-panel-ahead" title={mergeSummary(merge)} aria-label={mergeSummary(merge)}><vscode-icon name="arrow-up" aria-hidden="true" />{merge.unmerged}</span>}
      </div>
      <ul class="changes-panel-list" aria-label="Uncommitted files">
        {changes.files.map(file => {
          const label = statusLabel(file.status);
          const slash = file.path.lastIndexOf('/');
          const icon = file.status === 'modified' ? 'diff-modified' : file.status === 'deleted' ? 'diff-removed' : 'diff-added';
          return (
            <li key={file.path}>
              <button type="button" class={`changes-panel-row changes-panel-file changes-panel-${file.status}`} title={`${file.path} • ${label}`} aria-current={active.has(diffTabId(file.path)) ? 'true' : undefined} onClick={() => openDiffTab(store, file.path)}>
                <vscode-icon name={icon} class="changes-panel-status" role="img" aria-label={label} title={label} />
                <span class="changes-panel-name">{file.path.slice(slash + 1)}</span>
                {slash >= 0 && <span class="changes-panel-description">{file.path.slice(0, slash)}</span>}
                {file.added !== undefined && file.removed !== undefined && <span class="changes-panel-stat" aria-label={`${file.added} added, ${file.removed} removed`}><span>+{file.added}</span><span>−{file.removed}</span></span>}
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
