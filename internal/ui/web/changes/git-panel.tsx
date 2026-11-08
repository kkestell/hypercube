import {useEffect} from 'react';
import {Button} from '../components/ui/button';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {openDiffTab} from '../editor/editor.ts';
import {FileIcon} from '../files/file-icon.tsx';
import {currentSession} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {loadChanges, statusColor, statusLabel} from './changes.ts';
import {MergeButton} from './merge-button.tsx';

export function GitPanel() {
  const [state, store] = useStore();
  const changes = state.editor?.changes;
  const session = currentSession(state);
  useEffect(() => {
    if (state.editor && state.workbench.rightPanel && state.workbench.rightTab === 'git') loadChanges(store);
  }, [state.editor?.id, state.workbench.rightPanel, state.workbench.rightTab]);

  let message = '';
  if (!state.editor) message = 'Select a session or workspace to see its changes.';
  else if (!changes) message = 'No Git changes available.';
  else if ('missing' in changes) message = 'This folder no longer exists.';
  else if ('error' in changes) message = changes.error;
  else if (!changes.files.length) message = 'No uncommitted changes.';

  return <div className="git-panel h-full min-h-0 overflow-auto p-2 pt-1">
    {state.editor && changes && 'files' in changes ? <Tabs defaultValue="uncommitted">
      <TabsList className="w-full" aria-label="Change comparison">
        <TabsTrigger value="uncommitted">Uncommitted</TabsTrigger>
        <TabsTrigger value="base" disabled={!session?.worktree}>Branch changes</TabsTrigger>
      </TabsList>
      <TabsContent value="uncommitted">
        {message ? <p className="px-2 text-sm text-muted-foreground">{message}</p> : <ul aria-label="Git changes">
          {changes.files.map(change => {
            const counts = change.added !== undefined && change.removed !== undefined;
            return <li key={change.path}>
              <Button variant="ghost" size="sm" className="h-7 w-full min-w-0 justify-start px-2 font-normal"
                title={change.path} aria-label={`${change.path}, ${statusLabel(change.status)}${counts ? `, ${change.added} added, ${change.removed} removed` : ''}`}
                onClick={() => openDiffTab(store, change.path)}>
                <span className={`flex min-w-0 items-center gap-1.5 ${statusColor[change.status]}`}>
                  <FileIcon name={change.path} />
                  <span className={`truncate ${change.status === 'deleted' ? 'line-through' : ''}`}>{change.path}</span>
                </span>
                {counts && <span aria-hidden="true" className="ml-auto flex shrink-0 gap-1 text-xs text-muted-foreground tabular-nums">
                  <span>+{change.added}</span><span>−{change.removed}</span>
                </span>}
              </Button>
            </li>;
          })}
        </ul>}
      </TabsContent>
      <TabsContent value="base">
        <div className="space-y-3 px-2 py-1 text-sm">
          <p className="text-xs text-muted-foreground">Compared with <span className="font-medium text-foreground">{session?.merge?.target || 'base branch'}</span></p>
          {session && <MergeButton session={session} />}
          <p>All changes since branching, including uncommitted edits.</p>
          <p className="text-muted-foreground">UI preview — base comparison is not connected yet.</p>
        </div>
      </TabsContent>
    </Tabs> : <p className="px-2 text-sm text-muted-foreground">{message}</p>}
  </div>;
}
