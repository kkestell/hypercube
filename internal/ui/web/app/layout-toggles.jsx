// The layout toggles, which show and hide the side bar, the panel, and the
// right panel, as VS Code's do.
import {useStore} from '../store.js';
import {toggle} from './workbench.js';

const toggles = [
  {region: 'sidebar', icon: 'layout-sidebar-left', title: 'Toggle Primary Side Bar'},
  {region: 'panel', icon: 'layout-panel', title: 'Toggle Panel'},
  {region: 'rightPanel', icon: 'layout-sidebar-right', title: 'Toggle Secondary Side Bar'},
];

export function LayoutToggles() {
  const [state, store] = useStore();
  return (
    <vscode-toolbar-container class="layout-toggles">
      {toggles.map(({region, icon, title}) => {
        const shown = state.workbench[region];
        return (
          <vscode-toolbar-button
            key={region}
            id={`layout-toggle-${region}`}
            toggleable
            checked={shown}
            icon={shown ? icon : icon + '-off'}
            label={title}
            title={title}
            onChange={() => store.update(s => toggle(s, region))}
          />
        );
      })}
    </vscode-toolbar-container>
  );
}
