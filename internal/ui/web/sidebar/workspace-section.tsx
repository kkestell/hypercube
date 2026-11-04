import {useState} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Icon} from '../shared/icon.tsx';
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
  const [, store] = useStore();
  const [open, setOpen] = useState(true);
  const headingId = `workspace-section-${workspace.id}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className="workspace-section" data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className="workspace-section-header group/workspace flex items-center gap-1" title={workspace.path}>
        <CollapsibleTrigger asChild>
          <Button id={headingId} variant="ghost" size="sm" className="workspace-section-toggle h-7 min-w-0 flex-1 justify-start px-2 font-normal has-[>svg]:px-2 hover:bg-transparent dark:hover:bg-transparent" onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}>
            <span className="workspace-section-name min-w-0 truncate uppercase text-muted-foreground">{workspace.name}</span>
            <Icon name={open ? 'chevron-down' : 'chevron-right'} className="invisible group-hover/workspace:visible group-focus-within/workspace:visible" />
          </Button>
        </CollapsibleTrigger>
        {/* Keep the drawing layer stable when the fade reaches full opacity. */}
        <Button variant="ghost" size="icon-sm" className="size-7 opacity-0 will-change-[opacity] hover:bg-transparent dark:hover:bg-transparent group-hover/workspace:opacity-100 group-focus-within/workspace:opacity-100" aria-label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s, workspace.id))}><Icon name="add" /></Button>
        <Button variant="ghost" size="icon-sm" className="size-7 opacity-0 will-change-[opacity] hover:bg-transparent dark:hover:bg-transparent group-hover/workspace:opacity-100 group-focus-within/workspace:opacity-100" aria-label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)}><Icon name="ellipsis" /></Button>
      </div>
      <CollapsibleContent><SessionTree label={workspace.name} sessions={workspace.sessions} renaming={renaming} onRenaming={onRenaming} /></CollapsibleContent>
    </section>
  </Collapsible>;
}
