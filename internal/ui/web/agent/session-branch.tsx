// A worktree session's branch above the composer, right aligned, with a Merge button while
// the branch has commits not merged into the checkout's branch. Merge asks
// the session's agent to merge, so it is disabled while the session is
// stopped.
import {sessionOf} from '../sessions/selection.ts';
import {requestMerge} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {Button} from '../components/ui/button';
import {BranchIcon} from '../shared/branch-icon.tsx';
import {useStore} from '../store.ts';

export function SessionBranch({session}: {session: Session}) {
  const [state, store] = useStore();
  const {merge} = session;
  const found = sessionOf(state, session.id);
  if (!merge || !found) return null;

  return (
    <span className="composer-branch mb-3 flex min-w-0 items-center justify-end gap-2 text-xs text-muted-foreground">
      <BranchIcon merge={merge} iconClassName="size-3.5!" />
      <span className="composer-branch-name truncate">{merge.branch}</span>
      {merge.unmerged > 0 && !merge.error && (
        <Button type="button" variant="secondary" size="xs" className="composer-merge" disabled={!session.running}
          onClick={() => requestMerge(store, session, merge.target, found.workspace.path)}>
          Merge
        </Button>
      )}
    </span>
  );
}
