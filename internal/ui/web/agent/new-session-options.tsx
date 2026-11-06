// The new session names its workspace and explains its worktree choice.
import {Toggle} from '../components/ui/toggle';
import {Icon} from '../shared/icon.tsx';
import {unsentWorktree} from '../sessions/selection.ts';
import {useStore} from '../store.ts';

export function NewSessionOptions() {
  const [state, store] = useStore();
  const {unsent} = state;
  if (!unsent) return null;
  const workspace = state.workspaces.find(w => w.id === unsent.workspace);
  if (!workspace) return null;

  const toggle = (pressed: boolean) => {
    if (!unsent.sending) store.update(s => unsentWorktree(s, pressed));
  };

  return <div className="mb-3 space-y-2 px-3">
    <div className="min-w-0 text-sm">
      <div className="font-medium">{workspace.name}</div>
      <div className="truncate text-xs text-muted-foreground" title={workspace.path}>{workspace.path}</div>
    </div>
    {!workspace.noGit && <div className="space-y-1">
      <Toggle className="composer-worktree" size="sm" aria-label="Use a new worktree" title="Run in a new worktree"
        pressed={unsent.worktree} disabled={unsent.sending} onPressedChange={toggle}>
        <Icon name="git-branch" />Use a new worktree
      </Toggle>
      <p className="text-xs text-muted-foreground">Create a separate branch and working folder from the current checkout.</p>
    </div>}
  </div>;
}
