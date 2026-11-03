// The composer's choice to run a new top-level session in its own worktree.
// The unsent session can change it until its first prompt sends; an existing
// session shows its choice, disabled. Children and sessions outside Git show
// none.
import {sessionOf, unsentWorktree} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {unsentKey} from './drafts.js';

export function WorktreeCheckbox({session}) {
  const [state, store] = useStore();
  const unsent = session.id === unsentKey ? state.unsent : null;
  const workspace = unsent ? state.workspaces.find(w => w.id === unsent.workspace) : sessionOf(state, session.id)?.workspace;
  if (!workspace || workspace.noGit || session.parent) return null;

  const toggle = e => {
    if (unsent && !unsent.sending) store.update(s => unsentWorktree(s, e.currentTarget.checked));
  };

  return (
    <vscode-checkbox class="composer-worktree" label="Worktree" checked={unsent ? unsent.worktree : session.worktree}
      disabled={!unsent || unsent.sending} onChange={toggle} />
  );
}
