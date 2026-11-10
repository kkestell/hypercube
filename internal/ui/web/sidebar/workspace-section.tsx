// A workspace's heading and its session tree. The heading's name selects the
// workspace's dashboard. On hover or keyboard focus, a chevron to collapse or
// expand the sessions shows after the name, and New Session takes the place of
// the count of pull requests that need the user.
import {expandWorkspace} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {slide} from '../shared/motion.ts';
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
  const github = workspace.github;
  const needYou = github ? sortTurns(github.pullRequests, github.account).mine.length : 0;
  const needYouLabel = `${needYou} ${needYou === 1 ? 'pull request needs' : 'pull requests need'} you`;
  const headingId = `workspace-section-${workspace.id}`;
  const toggleLabel = `${open ? 'Collapse' : 'Expand'} ${workspace.name}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className={open && groups.length ? 'mb-2 last:mb-0' : ''} data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className="group/workspace flex items-center gap-1" title={workspace.path} data-workspace-header="">
        <Button id={headingId} aria-pressed={selected} variant={selected ? 'secondary' : 'ghost'} size="sm"
          className={`min-w-0 flex-1 justify-start px-2 font-normal ${selected ? 'hover:bg-secondary' : ''}`}
          onClick={() => store.update(s => selectWorkspace(s, workspace.id))} onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}>
          <span className="min-w-0 truncate text-xs uppercase text-muted-foreground">{workspace.name}</span>
        </Button>
        {groups.length > 0 && <CollapsibleTrigger asChild>
          <Button variant="ghost" size="icon-xs" className={`invisible text-muted-foreground opacity-0 ${shown}`} aria-label={toggleLabel} title={toggleLabel}><Chevron open={open} /></Button>
        </CollapsibleTrigger>}
        <div className="grid shrink-0 items-center justify-items-end">
          {needYou > 0 && <span className={`col-start-1 row-start-1 flex items-center gap-1 pr-1 text-xs text-muted-foreground ${hidden}`} title={needYouLabel} aria-label={needYouLabel} role="img">
            {needYou}<Icon name="git-pull-request" className="size-3" />
          </span>}
          <IconButton icon="add" label="New Session" className={`invisible col-start-1 row-start-1 opacity-0 ${shown}`} onClick={() => store.update(s => openUnsent(s, workspace.id))} />
        </div>
      </div>
      <CollapsibleContent className={slide}>
        <SessionTree label={workspace.name} groups={groups} />
      </CollapsibleContent>
    </section>
  </Collapsible>;
}
