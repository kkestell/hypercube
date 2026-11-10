// The directory and branch of the selected session or workspace, independent
// of the file tools. details go before the branch, and tracking inside it.
import type {ReactNode} from 'react';
import {editorDirectory} from '../editor/target.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';

export function WorkingContext({details, tracking}: {details?: ReactNode; tracking?: ReactNode}) {
  const [state, store] = useStore();
  const directory = editorDirectory(state);
  if (!directory || state.unsent) return null;
  const changes = state.editor?.changes;
  const branch = changes && 'files' in changes ? changes.branch : null;

  return <div aria-label="Working directory and branch" className="flex h-8 min-w-0 shrink-0 items-center gap-2 border-b px-3 text-xs text-muted-foreground">
    <span className="flex min-w-0 flex-1 items-center gap-1.5" title={directory}>
      <Icon name="folder" className="size-3.5" />
      <span className="truncate">{directory}</span>
    </span>
    {details}
    {branch !== null && <button type="button" className="flex min-w-0 max-w-1/2 items-center gap-1.5 hover:text-foreground"
      title={`Current branch: ${branch || 'Detached HEAD'}. Open Git changes.`}
      onClick={() => store.update(s => ({...s, workbench: {...s.workbench, rightPanel: true, rightTab: 'git'}}))}>
      <Icon name="git-branch" className="size-3.5" />
      <span className="truncate">{branch || 'Detached HEAD'}</span>
      {tracking}
    </button>}
  </div>;
}
