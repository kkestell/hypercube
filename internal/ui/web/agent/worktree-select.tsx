// The new-session page chooses whether a session in a Git workspace runs in
// the checkout, in a new worktree, or in a new worktree on a named branch.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.tsx';
import {inWorktree, unsentWorktree} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {NewBranchDialog} from './new-branch-dialog.tsx';

export function WorktreeSelect() {
  const [state, store] = useStore();
  const [naming, setNaming] = useState(false);
  const {unsent} = state;
  const workspace = unsent && state.workspaces.find(w => w.id === unsent.workspace);
  if (!unsent || !workspace || workspace.noGit) return null;
  const {branch} = unsent;
  const worktree = inWorktree(state, unsent);
  const choose = (worktree: boolean) => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, worktree));
  };
  const label = branch ? branch.name : worktree ? 'New worktree' : 'Existing checkout';
  return <div className="mb-2 pl-[11px]"><DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" size="sm" className="composer-worktree h-6 max-w-72 text-xs font-normal text-muted-foreground" aria-label="Location" disabled={unsent.sending}
        title={branch ? `New worktree on ${branch.name} from ${branch.start}` : undefined}>
        <Icon name={worktree ? 'git-branch' : 'laptop'} /><span className="truncate">{label}</span><Icon name="chevron-down" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="top" align="start" className="min-w-40">
      <DropdownMenuItem role="menuitemradio" aria-checked={!worktree} onSelect={() => choose(false)}>
        <Icon name="laptop" />Existing checkout{!worktree && <Icon name="check" className="ml-auto" />}
      </DropdownMenuItem>
      <DropdownMenuItem role="menuitemradio" aria-checked={worktree && !branch} onSelect={() => choose(true)}>
        <Icon name="git-branch" />New worktree{worktree && !branch && <Icon name="check" className="ml-auto" />}
      </DropdownMenuItem>
      <DropdownMenuItem role="menuitemradio" aria-checked={!!branch} onSelect={() => { if (!unsent.sending) setNaming(true); }}>
        <Icon name="git-branch" />New worktree on branch…{branch && <Icon name="check" className="ml-auto" />}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
  {naming && <NewBranchDialog workspace={workspace.id} current={branch} onClose={() => setNaming(false)}
    onSave={chosen => { store.update(s => unsentWorktree(s, true, chosen)); setNaming(false); }} />}
  </div>;
}
