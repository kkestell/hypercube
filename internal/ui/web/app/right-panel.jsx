// The right panel: the shown session's Files and Changes, as icon
// tabs.
import {ChangesPanel} from '../changes/changes-panel.jsx';
import {FilesPanel} from '../files/files-panel.jsx';
import {useStore} from '../store.js';
import {PanelTabs} from '../shared/panel-tabs.jsx';
import {selectRightTab} from './workbench.js';
import './right-panel.css';

const tabs = [
  {id: 'files', icon: 'files', title: 'Files'},
  {id: 'changes', icon: 'source-control', title: 'Changes'},
];

export function RightPanel() {
  const [state, store] = useStore();
  return (
    <aside class="right-panel">
      <PanelTabs label="Session views" tabs={tabs} selected={state.workbench.rightTab}
        onSelect={id => store.update(s => selectRightTab(s, id))}>
        <FilesPanel key={state.session?.id} />
        <ChangesPanel />
      </PanelTabs>
    </aside>
  );
}
