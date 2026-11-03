// The new session's choice to run in its own worktree, beside the profile.
// It shows only in a Git workspace and locks while the first prompt sends.
import {unsentWorktree} from '../sessions/selection.js';
import {useStore} from '../store.js';

export function WorktreeToggle() {
  const [state, store] = useStore();
  const {unsent} = state;
  const workspace = state.workspaces.find(w => w.id === unsent.workspace);
  if (!workspace || workspace.noGit) return null;

  const toggle = e => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, e.currentTarget.checked));
  };

  return (
    <vscode-checkbox toggle class="composer-worktree" label="Worktree" checked={unsent.worktree}
      disabled={unsent.sending} onChange={toggle} />
  );
}
