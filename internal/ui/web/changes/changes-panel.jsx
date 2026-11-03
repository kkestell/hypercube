// The Changes tab: a worktree session's branch, how it stands against the
// checkout's branch, and its uncommitted files. Checkout sessions show nothing.
import {mergeSummary, uncommittedSummary} from '../sessions/merge.js';
import {currentSession} from '../sessions/selection.js';
import {useStore} from '../store.js';
import './changes-panel.css';

export function ChangesPanel() {
  const [state] = useStore();
  const merge = currentSession(state)?.merge;
  if (!merge) return null;
  return (
    <div class="changes-panel">
      <p>{merge.branch}</p>
      <p>{mergeSummary(merge)}</p>
      {!merge.error && <p>{uncommittedSummary(merge)}</p>}
    </div>
  );
}
