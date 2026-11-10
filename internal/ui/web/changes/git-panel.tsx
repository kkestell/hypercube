import {useEffect} from 'react';
import {Button} from '../components/ui/button';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {openDiffTab, setGitComparison} from '../editor/editor.ts';
import {FileIcon} from '../files/file-icon.tsx';
import {currentSession} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {loadChanges, statusColor, statusLabel} from './changes.ts';
import {MergeButton} from './merge-button.tsx';
import type {ChangeComparison, Changes} from './types.ts';

function ChangeRows({changes, comparison}: {changes: Changes; comparison: ChangeComparison}) {
  const [, store] = useStore();
  let message = '';
  if (!changes) message = 'No Git changes available.';
  else if ('missing' in changes) message = 'This folder no longer exists.';
  else if ('error' in changes) message = changes.error;
  else if (!changes.files.length) message = comparison === 'branch' ? 'No committed changes.' : 'No uncommitted changes.';
  if (message) return <p className="px-2 text-sm text-muted-foreground">{message}</p>;
  if (!changes || !('files' in changes)) return null;
  return <ul aria-label="Git changes">{changes.files.map(change => {
    const counts = change.added !== undefined && change.removed !== undefined;
    return <li key={change.path}>
      <Button variant="ghost" size="sm" className="h-7 w-full min-w-0 justify-start px-2 font-normal"
        title={change.path} aria-label={`${change.path}, ${statusLabel(change.status)}${counts ? `, ${change.added} added, ${change.removed} removed` : ''}`}
        onClick={() => openDiffTab(store, change.path, comparison)}>
        <span className={`flex min-w-0 items-center gap-1.5 ${statusColor[change.status]}`}>
          <FileIcon name={change.path} />
          <span className={`truncate ${change.status === 'deleted' ? 'line-through' : ''}`}>{change.path}</span>
        </span>
        {counts && <span aria-hidden="true" className="ml-auto flex shrink-0 gap-1 text-xs text-muted-foreground tabular-nums">
          <span>+{change.added}</span><span>−{change.removed}</span>
        </span>}
      </Button>
    </li>;
  })}</ul>;
}

export function GitPanel() {
  const [state, store] = useStore();
  const editor = state.editor;
  const comparison = editor?.gitComparison ?? 'uncommitted';
  const session = currentSession(state);
  const ahead = session?.merge && !session.merge.error ? session.merge.unmerged : undefined;
  useEffect(() => {
    if (editor && state.workbench.rightPanel && state.workbench.rightTab === 'git') loadChanges(store, comparison);
  }, [editor?.id, state.editorVersion, comparison, state.workbench.rightPanel, state.workbench.rightTab]);
  const selectComparison = (value: string) => {
    if (value !== 'uncommitted' && value !== 'branch') return;
    store.update(state => setGitComparison(state, value));
  };
  return <div className="git-panel flex h-full min-h-0 flex-col">
    <div className="min-h-0 flex-1 overflow-auto p-2 pt-1">
      {!editor ? <p className="px-2 text-sm text-muted-foreground">Select a session or workspace to see its changes.</p>
        : !session?.worktree ? <ChangeRows changes={editor.changes} comparison="uncommitted" />
        : <Tabs value={comparison} onValueChange={selectComparison}>
        <TabsList className="w-full" aria-label="Change comparison">
          <TabsTrigger value="uncommitted">Uncommitted</TabsTrigger>
          <TabsTrigger value="branch">Committed</TabsTrigger>
        </TabsList>
        <TabsContent value="uncommitted"><ChangeRows changes={editor.changes} comparison="uncommitted" /></TabsContent>
        <TabsContent value="branch">
          {editor.branchChanges && 'files' in editor.branchChanges && ahead === 0 && !editor.branchChanges.files.length
            ? <p className="px-2 text-sm text-muted-foreground">No commits ahead of <span className="font-medium text-foreground">{editor.branchChanges.target}</span>.</p>
            : <>
              {editor.branchChanges && 'files' in editor.branchChanges && <p className="mb-2 border-b px-2 pt-1 pb-3 text-xs text-muted-foreground">
                {ahead === undefined ? 'Compared with' : `${ahead} ${ahead === 1 ? 'commit' : 'commits'} ahead of`} <span className="font-medium text-foreground">{editor.branchChanges.target}</span>
              </p>}
              <ChangeRows changes={editor.branchChanges} comparison="branch" />
            </>}
        </TabsContent>
      </Tabs>}
    </div>
    {editor && session?.worktree && <MergeButton session={session} />}
  </div>;
}
