// The new-session page chooses whether a session in a Git workspace runs in
// the checkout, in a new worktree, or in a new worktree on a named branch.
import {useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.tsx';
import {inWorktree, unsentWorktree} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {NewBranchDialog} from './new-branch-dialog.tsx';

export function WorktreeSelect() {
  const [state, store] = useStore();
  const [naming, setNaming] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const {unsent} = state;
  const workspace = unsent && state.workspaces.find(w => w.id === unsent.workspace);
  if (!unsent || !workspace || workspace.noGit) return null;
  const {branch} = unsent;
  const worktree = inWorktree(state, unsent);
  const choose = (worktree: boolean) => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, worktree));
  };
  const label = branch ? branch.name : worktree ? 'New worktree' : 'Existing checkout';
  return <div className="mb-2">
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button ref={trigger} type="button" variant="ghost" size="sm" className="h-6 max-w-72 text-xs font-normal text-muted-foreground" aria-label="Location" disabled={unsent.sending}
          title={branch ? `New worktree on ${branch.name} from ${branch.start}` : undefined}>
          <Icon name={worktree ? 'git-branch' : 'laptop'} /><span className="truncate">{label}</span><Icon name="chevron-down" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="min-w-40">
        <DropdownMenuRadioGroup value={branch ? 'branch' : worktree ? 'worktree' : 'checkout'}>
          <DropdownMenuRadioItem value="checkout" onSelect={() => choose(false)}><Icon name="laptop" />Existing checkout</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="worktree" onSelect={() => choose(true)}><Icon name="git-branch" />New worktree</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="branch" onSelect={() => { if (!unsent.sending) setNaming(true); }}><Icon name="git-branch" />New worktree on branch…</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
    <NewBranchDialog open={naming} workspace={workspace.id} current={branch} opener={trigger} onClose={() => setNaming(false)}
      onSave={chosen => { store.update(s => unsentWorktree(s, true, chosen)); setNaming(false); }} />
  </div>;
}
