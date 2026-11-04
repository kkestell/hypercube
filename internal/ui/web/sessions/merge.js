// How a worktree session's branch stands against the checkout's branch, as
// the Changes tab and the sidebar show it.

const count = (n, noun) => `${n} ${noun}${n === 1 ? '' : 's'}`;

// mergeSummary is the phrase for the branch's merge state.
export function mergeSummary(merge) {
  if (merge.error) return `Merge status unavailable: ${merge.error}`;
  if (merge.unmerged > 0) return `${count(merge.unmerged, 'commit')} not merged into ${merge.target}`;
  return merge.committed ? `Merged into ${merge.target}` : 'No commits yet';
}
