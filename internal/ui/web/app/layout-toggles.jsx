import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';
import {useStore} from '../store.js';
import {toggle} from './workbench.js';

const toggles = [
  {region: 'sidebar', icon: 'layout-sidebar-left', title: 'Toggle Primary Side Bar', shortcut: '⌘B'},
  {region: 'panel', icon: 'layout-panel', title: 'Toggle Panel', shortcut: '⌘J'},
  {region: 'rightPanel', icon: 'layout-sidebar-right', title: 'Toggle Secondary Side Bar', shortcut: '⌘⌥B'},
];
export function LayoutToggles() {
  const [state, store] = useStore();
  return <div className="layout-toggles flex items-center gap-1">
    {toggles.map(({region, icon, title, shortcut}) => <Button key={region} variant="ghost" size="icon"
      aria-label={title} title={`${title} (${shortcut})`} aria-pressed={state.workbench[region]}
      onClick={() => store.update(s => toggle(s, region))}><Icon name={icon} /></Button>)}
  </div>;
}
