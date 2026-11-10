// A workspace's heading and its session tree. The heading's name selects the
// workspace's dashboard. On hover or keyboard focus, a chevron to collapse or
// expand the sessions shows after the name, and New Session takes the place of
// the count of pull requests that need the user.
import {expandWorkspace} from '../app/workbench.ts';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron, Icon} from '../shared/icon.tsx';
import {IconButton} from '../shared/icon-button.tsx';
import {workspaceKey} from '../editor/target.ts';
import {sortTurns} from '../github/turns.ts';
import {openUnsent, selectWorkspace} from '../sessions/selection.ts';
import type {Workspace} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {SessionTree} from './session-tree.tsx';
import type {SessionGroup} from './sidebar-rows.ts';

const shown = 'transition-[opacity,visibility] group-hover/workspace:visible group-hover/workspace:opacity-100 group-has-[:focus-visible]/workspace:visible group-has-[:focus-visible]/workspace:opacity-100';
const hidden = 'transition-[opacity,visibility] group-hover/workspace:invisible group-hover/workspace:opacity-0 group-has-[:focus-visible]/workspace:invisible group-has-[:focus-visible]/workspace:opacity-0';

export function WorkspaceSection({workspace, groups}: {workspace: Workspace; groups: SessionGroup[]}) {
  const [state, store] = useStore();
  const open = !state.workbench.collapsedWorkspaces[workspace.id];
  const setOpen = (value: boolean) => store.update(s => expandWorkspace(s, workspace.id, value));
  const selected = state.selected === workspaceKey(workspace.id);
  const select = () => store.update(s => selectWorkspace(s, workspace.id));
  const github = workspace.github;
  const needYou = github ? sortTurns(github.pullRequests, github.account).mine.length : 0;
  const needYouLabel = `${needYou} ${needYou === 1 ? 'pull request needs' : 'pull requests need'} you`;
  const headingId = `workspace-section-${workspace.id}`;
  const toggleLabel = `${open ? 'Collapse' : 'Expand'} ${workspace.name}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className={open && groups.length ? 'mb-2 last:mb-0' : ''} data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className={`group/workspace flex h-7 items-center gap-2 rounded-md pr-1 pl-2 ${selected ? 'bg-secondary' : 'hover:bg-accent dark:hover:bg-accent/50'}`}
        title={workspace.path} data-workspace-header="" onClick={e => { if (e.target === e.currentTarget) select(); }}>
        <button type="button" id={headingId} aria-pressed={selected} className="min-w-0 truncate rounded-sm text-xs uppercase text-muted-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          onClick={select} onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}>{workspace.name}</button>
        {groups.length > 0 && <CollapsibleTrigger asChild>
          <button type="button" className={`invisible flex shrink-0 rounded-sm text-muted-foreground opacity-0 outline-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 ${shown}`} aria-label={toggleLabel} title={toggleLabel}><Chevron open={open} /></button>
        </CollapsibleTrigger>}
        <div className="ml-auto grid shrink-0 items-center justify-items-end" onClick={e => { if (e.target === e.currentTarget) select(); }}>
          {needYou > 0 && <span className={`col-start-1 row-start-1 flex items-center gap-1 pr-1 text-xs text-muted-foreground ${hidden}`} title={needYouLabel} aria-label={needYouLabel} role="img">
            {needYou}<Icon name="git-pull-request" className="size-3" />
          </span>}
          <IconButton icon="add" label="New Session" className={`invisible col-start-1 row-start-1 opacity-0 ${shown}`} onClick={() => store.update(s => openUnsent(s, workspace.id))} />
        </div>
      </div>
      <CollapsibleContent>
        <SessionTree label={workspace.name} groups={groups} />
      </CollapsibleContent>
    </section>
  </Collapsible>;
}
