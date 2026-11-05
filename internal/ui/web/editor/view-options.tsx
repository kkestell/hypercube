// The view options of a tab bar's active tab. Switching between preview and
// source is a button of its own. The other options are toggle buttons beside
// it while two options or fewer apply, and share a "…" menu otherwise.
import {toggleFixedWidth, toggleWrap} from '../app/workbench.ts';
import {firstKeys} from '../app/shortcuts.ts';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuShortcut, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Toggle} from '../components/ui/toggle';
import {setSource} from '../files/files.ts';
import {Icon, type IconName} from '../shared/icon.tsx';
import type {State} from '../state.ts';
import {useStore} from '../store.ts';
import type {TabEntry} from './types.ts';

interface Option {
  title: string;
  icon: IconName;
  keys?: string;
  on: boolean;
  toggle: (state: State) => State;
}

export function ViewOptions({active}: {active: TabEntry | undefined}) {
  const [state, store] = useStore();
  const file = active?.kind === 'file' ? active : null;
  const browserPreview = file && state.session?.files.open[file.path]?.preview && !file.source;
  const options: Option[] = [];
  if ((file && !browserPreview) || active?.kind === 'diff') {
    options.push({title: 'Word Wrap', icon: 'wrap', keys: firstKeys('wrap'), on: state.workbench.wrap, toggle: toggleWrap});
  }
  if (active?.kind === 'agent' || (file?.source === false && !browserPreview)) {
    options.push({title: 'Fixed Width', icon: 'fold-horizontal', on: state.workbench.fixedWidth, toggle: toggleFixedWidth});
  }
  const mode = file && file.source !== null && <Button className="size-7" variant="ghost" size="icon"
    aria-label={file.source ? 'Show Preview' : 'Show Source'} title={file.source ? 'Show Preview' : 'Show Source'}
    onClick={() => store.update(s => setSource(s, file.path, !file.source))}><Icon name={file.source ? 'open-preview' : 'go-to-file'} /></Button>;
  const menu = options.length > 1 && !!mode;
  return <>
    {mode}
    {menu ? <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button className="size-7" variant="ghost" size="icon" aria-label="View Options" title="View Options"><Icon name="ellipsis" /></Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {options.map(o => <DropdownMenuCheckboxItem key={o.title} checked={o.on} onCheckedChange={() => store.update(o.toggle)}>
          {o.title}{o.keys && <DropdownMenuShortcut>{o.keys}</DropdownMenuShortcut>}
        </DropdownMenuCheckboxItem>)}
      </DropdownMenuContent>
    </DropdownMenu> : options.map(o => <Toggle key={o.title} className="size-7 min-w-7" size="sm" aria-label={o.title}
      title={o.keys ? `${o.title} (${o.keys})` : o.title} pressed={o.on} onPressedChange={() => store.update(o.toggle)}>
      <Icon name={o.icon} />
    </Toggle>)}
  </>;
}
