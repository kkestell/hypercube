import {useState} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Icon} from '../shared/icon.jsx';
import {openUnsent} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {SessionTree} from './session-tree.jsx';

export function WorkspaceSection({workspace, onMenu, renaming, onRenaming}) {
  const [, store] = useStore();
  const [open, setOpen] = useState(true);
  const headingId = `workspace-section-${workspace.id}`;
  return <Collapsible open={open} onOpenChange={setOpen} asChild>
    <section className="workspace-section" data-workspace={workspace.id} aria-labelledby={headingId}>
      <div className="workspace-section-header flex items-center gap-1" title={workspace.path}>
        <CollapsibleTrigger asChild>
          <Button id={headingId} variant="ghost" className="workspace-section-toggle min-w-0 flex-1 justify-start" onKeyDown={e => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            e.preventDefault(); setOpen(e.key === 'ArrowRight');
          }}><Icon name={open ? 'chevron-down' : 'chevron-right'} /><span className="workspace-section-name truncate">{workspace.name}</span></Button>
        </CollapsibleTrigger>
        <Button variant="ghost" size="icon" aria-label="New Session" title="New Session" onClick={() => store.update(s => openUnsent(s, workspace.id))}><Icon name="add" /></Button>
        <Button variant="ghost" size="icon" aria-label="More Actions" title="More Actions" onClick={e => onMenu(e.currentTarget)}><Icon name="ellipsis" /></Button>
      </div>
      <CollapsibleContent><SessionTree label={workspace.name} sessions={workspace.sessions} renaming={renaming} onRenaming={onRenaming} /></CollapsibleContent>
    </section>
  </Collapsible>;
}
