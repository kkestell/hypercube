import {Button} from '../components/ui/button';
import {Icon, type IconName} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {toggle} from './workbench.ts';
import {firstKeys} from './shortcuts.ts';
import type {Region} from './types.ts';

const toggles: {region: Region; icon: IconName; title: string}[] = [
  {region: 'sidebar', icon: 'layout-sidebar-left', title: 'Toggle Primary Side Bar'},
  {region: 'panel', icon: 'layout-panel', title: 'Toggle Panel'},
  {region: 'rightPanel', icon: 'layout-sidebar-right', title: 'Toggle Secondary Side Bar'},
];
export function LayoutToggles() {
  const [state, store] = useStore();
  return <div className="layout-toggles flex items-center">
    {toggles.map(({region, icon, title}) => <Button key={region} className="size-6" variant="ghost" size="icon"
      aria-label={title} title={`${title} (${firstKeys(region)})`} aria-pressed={region === 'panel' ? state.editor?.panel ?? false : state.workbench[region]} disabled={region === 'panel' && !state.editor}
      onClick={() => store.update(s => toggle(s, region))}><Icon name={icon} /></Button>)}
  </div>;
}
