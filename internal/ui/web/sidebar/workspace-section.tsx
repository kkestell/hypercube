import {expandWorkspace} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron, Icon} from '../shared/icon.tsx';
import {openUnsent} from '../sessions/selection.ts';
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
  const headingId = `workspace-section-${workspace.id}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className={`workspace-section ${workspace.sessions.length ? 'mb-2 last:mb-0' : ''}`} data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className="workspace-section-header group/workspace relative flex items-center" title={workspace.path}>
        <CollapsibleTrigger asChild>
          <Button id={headingId} variant="ghost" size="sm" className="workspace-section-toggle h-7 min-w-0 flex-1 justify-start gap-2 px-2 font-normal has-[>svg]:px-2 hover:bg-transparent dark:hover:bg-transparent" onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}>
            <span className="workspace-section-name min-w-0 truncate text-[11px] uppercase text-muted-foreground">{workspace.name}</span>
            {workspace.sessions.length > 0 && <span className="invisible flex text-muted-foreground group-hover/workspace:visible group-has-[:focus-visible]/workspace:visible"><Chevron open={open} /></span>}
          </Button>
        </CollapsibleTrigger>
        <div className="invisible absolute inset-y-0 right-0 flex items-center bg-linear-to-r from-transparent via-sidebar via-25% to-sidebar pl-4 group-hover/workspace:visible group-has-[:focus-visible]/workspace:visible">
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
