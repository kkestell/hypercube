// While visible, refresh the shown Files tree and its Git changes, and each
// pane's active file or diff, once a second. An active diff also refreshes the
// changes so its tab title follows the current status. Requests run one at a
// time; overlapping polls are skipped.
import {loadChanges, readOpenDiff} from '../changes/changes.ts';
import {isDiffTab, tabPath} from '../editor/editor.ts';
import {panes} from '../editor/tab-layout.ts';
import type {Store} from '../store.ts';
import {loadTree, readOpenFile, shownFolders} from './files.ts';

export function createFilePoller(store: Store) {
  let running = false;
  return async function poll() {
    const {visible, session, workbench} = store.get();
    if (running || !visible || !session) return;
    running = true;
    try {
      const tree = workbench.rightPanel && workbench.rightTab === 'files';
      if (tree) {
        await loadTree(store, shownFolders(session.files));
      }
      const shownPanes = panes(session.tabs.root);
      const left = () => store.get().session?.id !== session.id;
      if (!left() && (tree || shownPanes.some(pane => isDiffTab(pane.active)))) await loadChanges(store);
      for (const pane of shownPanes) {
        if (left()) break;
        if (isDiffTab(pane.active)) await readOpenDiff(store, tabPath(pane.active));
        if (pane.active.startsWith('file:')) await readOpenFile(store, tabPath(pane.active));
      }
    } finally {
      running = false;
    }
  };
}

export function startFilePolling(store: Store) {
  setInterval(createFilePoller(store), 1000);
}
