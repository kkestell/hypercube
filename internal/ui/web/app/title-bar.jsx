// The title bar's layout controls, which show and hide the side bar, the
// panel, and the right panel, as VS Code's do.
import {useStore} from '../store.js';
import {toggle} from './workbench.js';
import './title-bar.css';

const toggles = [
  {region: 'sidebar', icon: 'layout-sidebar-left', title: 'Toggle Primary Side Bar'},
  {region: 'panel', icon: 'layout-panel', title: 'Toggle Panel'},
  {region: 'rightPanel', icon: 'layout-sidebar-right', title: 'Toggle Secondary Side Bar'},
];

export function TitleBar() {
  const [state, store] = useStore();
  return (
    <header class="title-bar">
      <vscode-toolbar-container class="title-bar-toggles">
        {toggles.map(({region, icon, title}) => {
          const shown = state.workbench[region];
          return (
            <vscode-toolbar-button
              key={region}
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
    </header>
  );
}
