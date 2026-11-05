// The "…" menu of a tab bar: the view options of its active tab.
import {toggleFixedWidth, toggleWrap} from '../app/workbench.ts';
import {barButton} from '../app/title-bar.ts';
import {firstKeys} from '../app/shortcuts.ts';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuShortcut, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {setSource} from '../files/files.ts';
import {Icon} from '../shared/icon.tsx';
import {useOverlay} from '../shared/overlays.ts';
import {useStore} from '../store.ts';
import type {TabEntry} from './types.ts';

export function ViewMenu({active}: {active: TabEntry | undefined}) {
  const [state, store] = useStore();
  const overlay = useOverlay();
  const file = active?.kind === 'file' ? active : null;
  const mode = file && file.source !== null;
  const wrap = active?.kind === 'file' || active?.kind === 'diff';
  const fixedWidth = active?.kind === 'agent' || file?.source === false;
  if (!mode && !wrap && !fixedWidth) return null;
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button className={`view-menu ${barButton}`} variant="ghost" size="icon" aria-label="View Options" title="View Options"><Icon name="ellipsis" /></Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent ref={overlay} className="view-menu-items" align="end">
      {mode && <DropdownMenuItem onSelect={() => store.update(s => setSource(s, file.path, !file.source))}>
        <Icon name={file.source ? 'open-preview' : 'go-to-file'} />{file.source ? 'Show Preview' : 'Show Source'}
      </DropdownMenuItem>}
      {wrap && <DropdownMenuCheckboxItem checked={state.workbench.wrap} onCheckedChange={() => store.update(toggleWrap)}>
        Word Wrap<DropdownMenuShortcut>{firstKeys('wrap')}</DropdownMenuShortcut>
      </DropdownMenuCheckboxItem>}
      {fixedWidth && <DropdownMenuCheckboxItem checked={state.workbench.fixedWidth} onCheckedChange={() => store.update(toggleFixedWidth)}>
        Fixed Width
      </DropdownMenuCheckboxItem>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
