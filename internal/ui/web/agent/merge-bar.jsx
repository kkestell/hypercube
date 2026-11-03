// The line above a worktree session's composer: its branch, how the branch
// stands against the checkout's branch, and a Merge button that asks the
// session's agent to merge while it has unmerged commits.
import {mergeSummary} from '../sessions/merge.js';
import {sessionOf} from '../sessions/selection.js';
import {requestMerge} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import './merge-bar.css';

export function MergeBar({session}) {
  const [state, store] = useStore();
  const {merge} = session;
  const workspace = sessionOf(state, session.id)?.workspace;
  const canMerge = merge.unmerged > 0 && !merge.error;

  return (
    <div class="merge-bar">
      <vscode-icon name="git-branch" />
      <span class="merge-bar-text">{merge.branch} · {mergeSummary(merge)}</span>
      {canMerge && (
        <vscode-button secondary class="merge-bar-merge" disabled={!session.running}
          onClick={() => requestMerge(store, session, merge.target, workspace.path)}>
          Merge
        </vscode-button>
      )}
    </div>
  );
}
