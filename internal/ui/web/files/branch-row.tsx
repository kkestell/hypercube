// The session's branch above the Files tree, with its unmerged commit count,
// or why its Git changes could not be read.
import {mergeSummary} from '../sessions/merge.ts';
import {currentSession} from '../sessions/selection.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';

export function BranchRow() {
  const [state] = useStore();
  const changes = state.session?.changes;
  if (!changes || 'missing' in changes) return null;
  if ('error' in changes) return <p className="branch-row-error px-4 pt-2 text-sm text-muted-foreground">{changes.error}</p>;
  const merge = currentSession(state)?.merge;
  return (
    <div className="branch-row flex items-center gap-2 px-4 pt-3 pb-1 text-sm">
      <Icon name="git-branch" aria-hidden="true" />
      <span className="branch-row-name min-w-0 flex-1 truncate">{changes.branch}</span>
      {merge && merge.unmerged > 0 && <span className="branch-row-ahead flex items-center gap-1 text-xs" role="img" title={mergeSummary(merge)} aria-label={mergeSummary(merge)}><Icon name="arrow-up" aria-hidden="true" />{merge.unmerged}</span>}
    </div>
  );
}
