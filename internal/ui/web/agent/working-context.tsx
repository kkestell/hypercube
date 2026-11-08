// The selected session's directory and branch, independent of the file tools.
import {Button} from '../components/ui/button';
import {editorDirectory} from '../editor/target.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';

export function WorkingContext() {
  const [state, store] = useStore();
  const directory = editorDirectory(state);
  if (!directory || state.unsent) return null;
  const changes = state.editor?.changes;
  const branch = changes && 'files' in changes ? changes.branch : null;

  return <div aria-label="Working directory and branch" className="flex h-8 min-w-0 shrink-0 items-center gap-2 border-b px-3 text-xs text-muted-foreground">
    <span className="min-w-0 flex-1 truncate" title={directory}>{directory}</span>
    {branch !== null && <Button variant="ghost" size="sm" className="h-6 min-w-0 max-w-1/2 px-1.5 text-xs font-normal"
      title={`Current branch: ${branch || 'Detached HEAD'}. Open Git changes.`}
      onClick={() => store.update(s => ({...s, workbench: {...s.workbench, rightPanel: true, rightTab: 'git'}}))}>
      <Icon name="git-branch" className="size-3.5" />
      <span className="truncate">{branch || 'Detached HEAD'}</span>
    </Button>}
  </div>;
}
