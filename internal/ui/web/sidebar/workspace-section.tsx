import {expandWorkspace} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron, Icon} from '../shared/icon.tsx';
import {workspaceKey} from '../editor/target.ts';
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
  const selected = state.selected === workspaceKey(workspace.id);
  const headingId = `workspace-section-${workspace.id}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className={`workspace-section ${workspace.sessions.length ? 'mb-2 last:mb-0' : ''}`} data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className="workspace-section-header group/workspace relative flex items-center" title={workspace.path}>
        <Button id={headingId} aria-pressed={selected} onClick={() => store.update(s => selectWorkspace(s, workspace.id))} variant={selected ? 'secondary' : 'ghost'} size="sm" className="workspace-section-toggle h-7 min-w-0 flex-1 justify-start gap-2 px-2 font-normal has-[>svg]:px-2 hover:bg-transparent dark:hover:bg-transparent">
          <span className="workspace-section-name min-w-0 truncate text-[11px] uppercase text-muted-foreground">{workspace.name}</span>
        </Button>
        <div className="invisible absolute inset-y-0 right-0 flex items-center bg-linear-to-r from-transparent via-sidebar via-25% to-sidebar pl-4 group-hover/workspace:visible group-has-[:focus-visible]/workspace:visible">
          <CollapsibleTrigger asChild><Button variant="ghost" size="icon-sm" className="size-6" aria-label={`${open ? 'Collapse' : 'Expand'} ${workspace.name}`} title={`${open ? 'Collapse' : 'Expand'} ${workspace.name}`}><Chevron open={open} /></Button></CollapsibleTrigger>
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
