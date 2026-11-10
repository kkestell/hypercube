// The icon of a worktree session's branch: its shape and color say whether the
// branch has uncommitted files, commits not in the target, or nothing to do.
import type {Merge} from '../sessions/types.ts';
import {branchState, branchSummary, type BranchState} from '../sessions/merge.ts';
import type {IconName} from './icon.tsx';
import {StatusIcon} from './status-icon.tsx';
import {tones} from './tones.ts';

const looks: Record<BranchState, {icon: IconName; color: string}> = {
  error: {icon: 'warning', color: tones.error},
  uncommitted: {icon: 'git-branch', color: tones.warning},
  unmerged: {icon: 'git-pull-request-arrow', color: tones.success},
  merged: {icon: 'git-merge', color: tones.muted},
  empty: {icon: 'git-branch', color: tones.muted},
};

export function BranchIcon({merge, className}: {merge?: Merge; className?: string}) {
  const {icon, color} = looks[merge ? branchState(merge) : 'empty'];
  return <StatusIcon icon={icon} color={color} title={merge ? `${merge.branch}: ${branchSummary(merge)}` : 'Worktree'} className={className} />;
}
