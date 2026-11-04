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
