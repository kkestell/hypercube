// The new-session page chooses whether a session in a Git workspace runs in
// the checkout or in a new worktree.
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.tsx';
import {unsentWorktree} from '../sessions/selection.ts';
import {useStore} from '../store.ts';

const choices = [
  {value: 'local', icon: 'laptop', label: 'Local'},
  {value: 'worktree', icon: 'git-branch', label: 'Worktree'},
] as const;

export function WorktreeSelect() {
  const [state, store] = useStore();
  const {unsent} = state;
  const workspace = unsent && state.workspaces.find(w => w.id === unsent.workspace);
  if (!unsent || !workspace || workspace.noGit) return null;
  const shown = choices[unsent.worktree ? 1 : 0];
  const choose = (value: string) => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, value === 'worktree'));
  };
  return <div className="mb-2 pl-[11px]"><DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" size="sm" className="composer-worktree h-6 text-xs font-normal text-muted-foreground" aria-label="Location" disabled={unsent.sending}>
        <Icon name={shown.icon} />{shown.label}<Icon name="chevron-down" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="top" align="start" className="min-w-40">
      {choices.map(c => <DropdownMenuItem key={c.value} role="menuitemradio" aria-checked={c === shown} onSelect={() => choose(c.value)}>
        <Icon name={c.icon} />{c.label}
        {c === shown && <Icon name="check" className="ml-auto" />}
      </DropdownMenuItem>)}
    </DropdownMenuContent>
  </DropdownMenu></div>;
}
