// A worktree session's branch beside the profile, with a Merge button while
// the branch has commits not merged into the checkout's branch. Merge asks
// the session's agent to merge, so it is disabled while the session is
// stopped.
import {sessionOf} from '../sessions/selection.ts';
import {requestMerge} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';

export function SessionBranch({session}: {session: Session}) {
  const [state, store] = useStore();
  const {merge} = session;
  const found = sessionOf(state, session.id);
  if (!merge || !found) return null;

  return (
    <span className="composer-branch flex min-w-0 items-center gap-2">
      <Icon name="git-branch" />
      <span className="composer-branch-name truncate">{merge.branch}</span>
      {merge.unmerged > 0 && !merge.error && (
        <Button type="button" variant="secondary" size="sm" className="composer-merge" disabled={!session.running}
          onClick={() => requestMerge(store, session, merge.target, found.workspace.path)}>
          Merge
        </Button>
      )}
    </span>
  );
}
