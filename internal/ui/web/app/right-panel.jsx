// The right panel: the shown session's Files and Changes, as icon
// tabs. Changes is empty.
import {FilesPanel} from '../files/files-panel.jsx';
import {useStore} from '../store.js';
import '../shared/panel-tabs.css';
import {selectRightTab} from './workbench.js';
import './right-panel.css';

const tabs = [
  {id: 'files', icon: 'files', title: 'Files'},
  {id: 'changes', icon: 'source-control', title: 'Changes'},
];

export function RightPanel() {
  const [state, store] = useStore();
  const selected = tabs.findIndex(tab => tab.id === state.workbench.rightTab);
  return (
    <aside class="right-panel">
      <vscode-tabs class="panel-tabs" panel selected-index={selected}
        onvsc-tabs-select={e => store.update(s => selectRightTab(s, tabs[e.detail.selectedIndex].id))}>
        {tabs.map(tab => (
          <vscode-tab-header key={tab.id} slot="header" title={tab.title} aria-label={tab.title}>
            <vscode-icon name={tab.icon} />
          </vscode-tab-header>
        ))}
        <vscode-tab-panel><FilesPanel key={state.session?.id} /></vscode-tab-panel>
        <vscode-tab-panel />
      </vscode-tabs>
    </aside>
  );
}
