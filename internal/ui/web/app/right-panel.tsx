// The right panel: the current view's Files, Git, and Search, and every signed-in
// account's pull requests, as icon tabs. The Pull Requests tab shows how many
// wait on the user, and only while an account is signed in. Its list mounts
// only while shown, so showing the tab does not replay its groups' opening.
import {GitPanel} from '../changes/git-panel.tsx';
import {SearchPanel} from '../files/search-panel.tsx';
import {FilesPanel} from '../files/files-panel.tsx';
import {AllPullRequests} from '../github/all-pull-requests.tsx';
import {yourMoves} from '../github/turns.ts';
import {useStore} from '../store.ts';
import {PanelTabs, type PanelTab} from '../shared/panel-tabs.tsx';
import {selectRightTab} from './workbench.ts';
import type {RightTab} from './types.ts';

const fileTabs: PanelTab<RightTab>[] = [
  {id: 'files', icon: 'files', title: 'Files'},
  {id: 'git', icon: 'source-control', title: 'Git'},
  {id: 'search', icon: 'search', title: 'Search'},
];

export function RightPanel() {
  const [state, store] = useStore();
  const signedIn = state.accounts.length > 0;
  const tabs = signedIn ? [...fileTabs, {id: 'pullRequests' as const, icon: 'git-pull-request' as const, title: 'Pull Requests', count: yourMoves(state.accounts).length}] : fileTabs;
  const selected = tabs.some(tab => tab.id === state.workbench.rightTab) ? state.workbench.rightTab : 'files';
  return (
    <aside className="@container h-full min-w-0 bg-sidebar" data-region="rightPanel">
      <PanelTabs label="Tools" tabs={tabs} selected={selected}
        onSelect={id => store.update(s => selectRightTab(s, id))}>
        <FilesPanel key={state.editor?.id} />
        <GitPanel key={state.editor?.id} />
        <SearchPanel key={state.editor?.id} />
        {selected === 'pullRequests' && <AllPullRequests />}
      </PanelTabs>
    </aside>
  );
}
