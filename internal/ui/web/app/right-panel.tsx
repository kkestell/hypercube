// The right panel: the current view's Files, Git, and Search, as icon tabs.
import {GitPanel} from '../changes/git-panel.tsx';
import {SearchPanel} from '../files/search-panel.tsx';
import {FilesPanel} from '../files/files-panel.tsx';
import {useStore} from '../store.ts';
import {PanelTabs, type PanelTab} from '../shared/panel-tabs.tsx';
import {selectRightTab} from './workbench.ts';
import type {RightTab} from './types.ts';

const tabs: PanelTab<RightTab>[] = [
  {id: 'files', icon: 'files', title: 'Files'},
  {id: 'git', icon: 'source-control', title: 'Git'},
  {id: 'search', icon: 'search', title: 'Search'},
];

export function RightPanel() {
  const [state, store] = useStore();
  return (
    <aside className="right-panel h-full min-w-0 bg-sidebar">
      <PanelTabs label="File tools" tabs={tabs} selected={state.workbench.rightTab}
        onSelect={id => store.update(s => selectRightTab(s, id))}>
        <FilesPanel key={state.editor?.id} />
        <GitPanel key={state.editor?.id} />
        <SearchPanel key={state.editor?.id} />
      </PanelTabs>
    </aside>
  );
}
