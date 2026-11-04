// A worktree session's branch beside the profile, with a Merge button while
// the branch has commits not merged into the checkout's branch. Merge asks
// the session's agent to merge, so it is disabled while the session is
// stopped.
import {sessionOf} from '../sessions/selection.js';
import {requestMerge} from '../sessions/session-actions.js';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';
import {useStore} from '../store.js';

export function SessionBranch({session}) {
  const [state, store] = useStore();
  const {merge} = session;
  if (!merge) return null;
  const workspace = sessionOf(state, session.id)?.workspace;

  return (
    <span className="composer-branch flex min-w-0 items-center gap-2">
      <Icon name="git-branch" />
      <span className="composer-branch-name truncate">{merge.branch}</span>
      {merge.unmerged > 0 && !merge.error && (
        <Button type="button" variant="secondary" size="sm" className="composer-merge" disabled={!session.running}
          onClick={() => requestMerge(store, session, merge.target, workspace.path)}>
          Merge
        </Button>
      )}
    </span>
  );
}
