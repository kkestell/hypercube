// A worktree session's branch beside the profile, with a Merge button while
// the branch has commits not merged into the checkout's branch. Merge asks
// the session's agent to merge, so it is disabled while the session is
// stopped.
import {sessionOf} from '../sessions/selection.js';
import {requestMerge} from '../sessions/session-actions.js';
import {useStore} from '../store.js';

export function SessionBranch({session}) {
  const [state, store] = useStore();
  const {merge} = session;
  if (!merge) return null;
  const workspace = sessionOf(state, session.id)?.workspace;

  return (
    <span class="composer-branch">
      <vscode-icon name="git-branch" />
      <span class="composer-branch-name">{merge.branch}</span>
      {merge.unmerged > 0 && !merge.error && (
        <vscode-button secondary class="composer-merge" disabled={!session.running}
          onClick={() => requestMerge(store, session, merge.target, workspace.path)}>
          Merge
        </vscode-button>
      )}
    </span>
  );
}
