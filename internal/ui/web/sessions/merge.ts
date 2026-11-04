// How a worktree session's branch stands against the checkout's branch, as
// the Changes tab and the sidebar show it.
import type {Merge} from './types.ts';

const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`;

// mergeSummary is the phrase for the branch's merge state.
export function mergeSummary(merge: Merge) {
  if (merge.error) return `Merge status unavailable: ${merge.error}`;
  if (merge.unmerged > 0) return `${count(merge.unmerged, 'commit')} not merged into ${merge.target}`;
  return merge.committed ? `Merged into ${merge.target}` : 'No commits yet';
}

export type BranchState = 'error' | 'uncommitted' | 'unmerged' | 'merged' | 'empty';

// branchState is the branch's state in order of attention: an unmeasurable
// branch, uncommitted files, commits not in the target, a merged branch, or
// a branch with no commits.
export function branchState(merge: Merge): BranchState {
  if (merge.error) return 'error';
  if (merge.uncommitted > 0) return 'uncommitted';
  if (merge.unmerged > 0) return 'unmerged';
  return merge.committed ? 'merged' : 'empty';
}

// branchSummary is mergeSummary with the count of uncommitted files first.
export function branchSummary(merge: Merge) {
  if (merge.error || merge.uncommitted === 0) return mergeSummary(merge);
  const rest = mergeSummary(merge);
  return `${count(merge.uncommitted, 'uncommitted file')}, ${rest[0].toLowerCase()}${rest.slice(1)}`;
}
