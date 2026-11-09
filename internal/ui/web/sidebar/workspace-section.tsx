import {expandWorkspace} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron, Icon} from '../shared/icon.tsx';
import {workspaceKey} from '../editor/target.ts';
import {sortTurns} from '../github/turns.ts';
import {openUnsent, selectWorkspace} from '../sessions/selection.ts';
import type {Workspace} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {SessionTree} from './session-tree.tsx';

interface WorkspaceSectionProps {
  workspace: Workspace;
  onMenu: (button: HTMLElement) => void;
  renaming: string | null;
  onRenaming: (id: string | null) => void;
}

export function WorkspaceSection({workspace, onMenu, renaming, onRenaming}: WorkspaceSectionProps) {
  const [state, store] = useStore();
  const open = !state.workbench.collapsedWorkspaces[workspace.id];
  const setOpen = (value: boolean) => store.update(s => expandWorkspace(s, workspace.id, value));
  const select = () => store.update(s => selectWorkspace(s, workspace.id));
  const selected = state.selected === workspaceKey(workspace.id);
  // The row and its action fade share one surface in every state. Compose
  // dark hover over the sidebar once so the fade stays opaque over the name.
  const surface = selected ? '[--workspace-surface:var(--secondary)]'
    : '[--workspace-surface:var(--sidebar)] hover:[--workspace-surface:var(--accent)] dark:hover:[--workspace-surface:color-mix(in_srgb,var(--accent)_50%,var(--sidebar))]';
  const github = workspace.github;
  const needYou = github ? sortTurns(github.pullRequests, github.account).mine.length : 0;
  const needYouLabel = `${needYou} ${needYou === 1 ? 'pull request needs' : 'pull requests need'} you`;
  const headingId = `workspace-section-${workspace.id}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className={`workspace-section ${open && workspace.sessions.length ? 'mb-2 last:mb-0' : ''}`} data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className={`workspace-section-header group/workspace relative flex items-center rounded-md bg-[var(--workspace-surface)] ${surface}`} title={workspace.path} onClick={e => {
        if (e.target instanceof Element && e.target.closest('button')) return;
        select();
      }}>
        <div className="flex min-w-0 flex-1 items-center px-2">
          <Button id={headingId} aria-pressed={selected} onClick={select} variant="ghost" size="sm" className="workspace-section-toggle h-7 min-w-0 shrink justify-start px-0 font-normal hover:bg-transparent dark:hover:bg-transparent" onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}>
            <span className="workspace-section-name min-w-0 truncate text-[11px] uppercase text-muted-foreground">{workspace.name}</span>
          </Button>
          {needYou > 0 && <span className="ml-1.5 flex shrink-0 items-center gap-0.5 text-[11px] text-muted-foreground" title={needYouLabel} aria-label={needYouLabel} role="img">
            <Icon name="git-pull-request" className="size-3" />{needYou}
          </span>}
          {workspace.sessions.length > 0 && <CollapsibleTrigger asChild>
            <Button variant="ghost" size="icon-xs" className="invisible ml-2 size-4 text-muted-foreground hover:bg-transparent dark:hover:bg-transparent group-hover/workspace:visible group-has-[:focus-visible]/workspace:visible" aria-label={`${open ? 'Collapse' : 'Expand'} ${workspace.name}`} title={`${open ? 'Collapse' : 'Expand'} ${workspace.name}`}><Chevron open={open} /></Button>
          </CollapsibleTrigger>}
        </div>
        <div className="invisible absolute inset-y-0 right-0 flex items-center rounded-r-md bg-linear-to-r from-transparent via-[var(--workspace-surface)] via-25% to-[var(--workspace-surface)] pl-4 group-hover/workspace:visible group-has-[:focus-visible]/workspace:visible">
          <Button variant="ghost" size="icon-sm" className="size-6" aria-label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s, workspace.id))}><Icon name="add" /></Button>
          <Button variant="ghost" size="icon-sm" className="size-6" aria-label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)}><Icon name="ellipsis" /></Button>
        </div>
      </div>
      <CollapsibleContent>
        <SessionTree label={workspace.name} sessions={workspace.sessions} renaming={renaming} onRenaming={onRenaming} />
      </CollapsibleContent>
    </section>
  </Collapsible>;
}
