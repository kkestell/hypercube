// The view options of a tab bar's active tab. Switching between preview and
// source is a button of its own. The other options are toggle buttons beside
// it while two options or fewer apply, and share a "…" menu otherwise.
import {toggleFixedWidth, toggleWrap} from '../app/workbench.ts';
import {firstKeys, type ShortcutId} from '../app/shortcuts.ts';
import {DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuShortcut, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {setSource} from '../files/files.ts';
import type {IconName} from '../shared/icon.tsx';
import {IconButton, pressable} from '../shared/icon-button.tsx';
import type {State} from '../state.ts';
import {useStore} from '../store.ts';
import type {TabEntry} from './types.ts';

interface Option {
  title: string;
  icon: IconName;
  shortcut?: ShortcutId;
  on: boolean;
  toggle: (state: State) => State;
}

export function ViewOptions({active}: {active: TabEntry | undefined}) {
  const [state, store] = useStore();
  const file = active?.kind === 'file' ? active : null;
  const browserPreview = file && state.editor?.files.open[file.path]?.preview && !file.source;
  const options: Option[] = [];
  if ((file && !browserPreview) || active?.kind === 'diff') {
    options.push({title: 'Word Wrap', icon: 'wrap', shortcut: 'wrap', on: state.workbench.wrap, toggle: toggleWrap});
  }
  if (file?.source === false && !browserPreview) {
    options.push({title: 'Fixed Width', icon: 'fold-horizontal', on: state.workbench.fixedWidth, toggle: toggleFixedWidth});
  }
  const mode = file && file.source !== null && <IconButton icon={file.source ? 'open-preview' : 'go-to-file'} label={file.source ? 'Show Preview' : 'Show Source'}
    onClick={() => store.update(s => setSource(s, file.path, !file.source))} />;
  const menu = options.length > 1 && !!mode;
  return <>
    {mode}
    {menu ? <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <IconButton icon="ellipsis" label="View Options" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {options.map(o => <DropdownMenuCheckboxItem key={o.title} checked={o.on} onCheckedChange={() => store.update(o.toggle)}>
          {o.title}{o.shortcut && <DropdownMenuShortcut>{firstKeys(o.shortcut)}</DropdownMenuShortcut>}
        </DropdownMenuCheckboxItem>)}
      </DropdownMenuContent>
    </DropdownMenu> : options.map(o => <IconButton key={o.title} icon={o.icon} label={o.title} shortcut={o.shortcut}
      className={pressable} aria-pressed={o.on} onClick={() => store.update(o.toggle)} />)}
  </>;
}
