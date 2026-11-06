// The right panel: the shown session's Files and Search, as icon tabs.
import {SearchPanel} from '../files/search-panel.tsx';
import {FilesPanel} from '../files/files-panel.tsx';
import {useStore} from '../store.ts';
import {PanelTabs, type PanelTab} from '../shared/panel-tabs.tsx';
import {selectRightTab} from './workbench.ts';
import type {RightTab} from './types.ts';

const tabs: PanelTab<RightTab>[] = [
  {id: 'files', icon: 'files', title: 'Files'},
  {id: 'search', icon: 'search', title: 'Search'},
];

export function RightPanel() {
  const [state, store] = useStore();
  return (
    <aside className="right-panel h-full min-w-0 bg-sidebar">
      <PanelTabs label="Session views" tabs={tabs} selected={state.workbench.rightTab}
        onSelect={id => store.update(s => selectRightTab(s, id))}>
        <FilesPanel key={state.session?.id} />
        <SearchPanel key={state.session?.id} />
      </PanelTabs>
    </aside>
  );
}
