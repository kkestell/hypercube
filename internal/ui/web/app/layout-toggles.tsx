// The buttons that show and hide the sidebar, the terminal, and the right
// panel, named as their shortcuts are.
import {IconButton, pressable} from '../shared/icon-button.tsx';
import type {IconName} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {shortcutTitle} from './shortcuts.ts';
import type {Region} from './types.ts';
import {toggle} from './workbench.ts';

const icons: Record<Region, IconName> = {sidebar: 'layout-sidebar-left', panel: 'layout-panel', rightPanel: 'layout-sidebar-right'};

export function LayoutToggles() {
  const [state, store] = useStore();
  return <div className="flex items-center">
    {(['sidebar', 'panel', 'rightPanel'] as const).map(region => <IconButton key={region} icon={icons[region]} label={shortcutTitle(region)} shortcut={region}
      className={pressable} aria-pressed={region === 'panel' ? state.editor?.panel ?? false : state.workbench[region]} disabled={region === 'panel' && !state.editor}
      onClick={() => store.update(s => toggle(s, region))} />)}
  </div>;
}
