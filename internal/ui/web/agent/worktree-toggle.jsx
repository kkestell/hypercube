// The new session's choice to run in its own worktree, beside the profile.
// It shows only in a Git workspace and locks while the first prompt sends.
import {Toggle} from '../components/ui/toggle';
import {Icon} from '../shared/icon.jsx';
import {unsentWorktree} from '../sessions/selection.js';
import {useStore} from '../store.js';

export function WorktreeToggle() {
  const [state, store] = useStore();
  const {unsent} = state;
  if (!unsent) return null;
  const workspace = state.workspaces.find(w => w.id === unsent.workspace);
  if (!workspace || workspace.noGit) return null;

  const toggle = pressed => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, pressed));
  };

  return <Toggle className="composer-worktree" size="sm" aria-label="Worktree" title="Run in a worktree"
    pressed={!!unsent.worktree} disabled={unsent.sending} onPressedChange={toggle}>
    <Icon name="git-branch" />
  </Toggle>;
}
