// The icon of a worktree session's branch: its shape and color say whether the
// branch has uncommitted files, commits not in the target, or nothing to do.
import type {Merge} from '../sessions/types.ts';
import {branchState, branchSummary, type BranchState} from '../sessions/merge.ts';
import {Icon, type IconName} from './icon.tsx';

const looks: Record<BranchState, {icon: IconName; color: string}> = {
  error: {icon: 'warning', color: 'text-destructive'},
  uncommitted: {icon: 'git-branch', color: 'text-amber-600 dark:text-amber-500'},
  unmerged: {icon: 'git-pull-request-arrow', color: 'text-green-600 dark:text-green-500'},
  merged: {icon: 'git-merge', color: 'text-muted-foreground'},
  empty: {icon: 'git-branch', color: 'text-muted-foreground'},
};

export function BranchIcon({merge, className = ''}: {merge?: Merge; className?: string}) {
  const {icon, color} = looks[merge ? branchState(merge) : 'empty'];
  const title = merge ? `${merge.branch}: ${branchSummary(merge)}` : 'Worktree';
  return <span className={`inline-flex ${className}`} title={title}>
    <Icon name={icon} className={color} title={title} role="img" />
  </span>;
}
