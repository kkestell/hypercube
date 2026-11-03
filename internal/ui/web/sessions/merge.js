// How a worktree session's branch stands against the checkout's branch, as
// the merge bar and the sidebar show it.

// mergeSummary is the bar's phrase for the branch's merge state.
export function mergeSummary(merge) {
  if (merge.error) return `Merge status unavailable: ${merge.error}`;
  const state = merge.unmerged > 0
    ? `${merge.unmerged} ${merge.unmerged === 1 ? 'commit' : 'commits'} not merged into ${merge.target}`
    : merge.committed ? `Merged into ${merge.target}` : 'No commits yet';
  if (merge.uncommitted === 0) return state;
  return `${state} · ${merge.uncommitted} uncommitted ${merge.uncommitted === 1 ? 'file' : 'files'}`;
}

// mergeTint is 'modified' while the branch has work outside the target,
// 'merged' once its commits are all in it, and otherwise null.
export function mergeTint(merge) {
  if (!merge || merge.error) return null;
  if (merge.unmerged > 0 || merge.uncommitted > 0) return 'modified';
  return merge.committed ? 'merged' : null;
}
