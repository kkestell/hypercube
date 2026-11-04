import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';
// The session's repository branch and flat list of uncommitted files.
import {useEffect} from 'react';
import {diffTabId, openDiffTab} from '../editor/editor.js';
import {panes} from '../editor/tab-layout.js';
import {mergeSummary} from '../sessions/merge.js';
import {currentSession} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {loadChanges, statusLabel} from './changes.js';

export function ChangesPanel() {
  const [state, store] = useStore();
  const session = state.session;
  useEffect(() => { loadChanges(store); }, [session?.id]);
  const changes = session?.changes;
  if (!changes) return null;
  if (changes.missing || changes.error) return <div className="changes-panel-message p-4 text-muted-foreground">{changes.missing ? 'The folder no longer exists.' : changes.error}</div>;
  const merge = currentSession(state)?.merge;
  const active = new Set(panes(session.tabs.root).map(pane => pane.active));
  return (
    <div className="changes-panel p-2">
      <div className="panel-row changes-panel-row changes-panel-branch flex items-center gap-2 p-2 text-sm">
        <Icon name="git-branch" className="panel-row-icon" aria-hidden="true" />
        <span className="changes-panel-details flex min-w-0 flex-1 flex-col items-start"><span className="changes-panel-name truncate">{changes.branch}</span></span>
        {merge?.unmerged > 0 && <span className="changes-panel-ahead changes-panel-stat flex items-center gap-1 text-xs" role="img" title={mergeSummary(merge)} aria-label={mergeSummary(merge)}><Icon name="arrow-up" aria-hidden="true" />{merge.unmerged}</span>}
      </div>
      <ul className="changes-panel-list" aria-label="Uncommitted files">
        {changes.files.map(file => {
          const label = statusLabel(file.status);
          const slash = file.path.lastIndexOf('/');
          const icon = file.status === 'modified' ? 'diff-modified' : file.status === 'deleted' ? 'diff-removed' : 'diff-added';
          return (
            <li key={file.path}>
              <Button variant="ghost" type="button" className={`changes-panel-row changes-panel-file changes-panel-${file.status} h-auto w-full justify-between py-2`} title={`${file.path} • ${label}`} aria-current={active.has(diffTabId(file.path)) ? 'true' : undefined} onClick={() => openDiffTab(store, file.path)}>
                <span className="changes-panel-entry flex min-w-0 items-center gap-2">
                  <Icon name={icon} className="panel-row-icon changes-panel-status" role="img" aria-label={label} title={label} />
                  <span className="changes-panel-details flex min-w-0 flex-1 flex-col items-start">
                    <span className="changes-panel-name truncate">{file.path.slice(slash + 1)}</span>
                    {slash >= 0 && <span className="changes-panel-description truncate text-xs text-muted-foreground">{file.path.slice(0, slash)}</span>}
                  </span>
                </span>
                {file.added !== undefined && file.removed !== undefined && <span className="changes-panel-stat flex gap-1 text-xs" aria-label={`${file.added} added, ${file.removed} removed`}><span>+{file.added}</span><span>−{file.removed}</span></span>}
              </Button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
