// The new session's choice to run in its own worktree, beside the profile.
// It shows only in a Git workspace and locks while the first prompt sends.
import {Checkbox} from '../components/ui/checkbox';
import {Label} from '../components/ui/label';
import {unsentWorktree} from '../sessions/selection.js';
import {useStore} from '../store.js';

export function WorktreeToggle() {
  const [state, store] = useStore();
  const {unsent} = state;
  if (!unsent) return null;
  const workspace = state.workspaces.find(w => w.id === unsent.workspace);
  if (!workspace || workspace.noGit) return null;

  const toggle = checked => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, checked));
  };

  return <Label className="composer-worktree flex items-center gap-2">
    <Checkbox aria-label="Worktree" checked={!!unsent.worktree} disabled={unsent.sending} onCheckedChange={toggle} />Worktree
  </Label>;
}
