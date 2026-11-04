// While visible, refresh the shown Files or Changes list and each pane's
// active file or diff once a second. An active diff also refreshes Changes
// so its tab title follows the current status. Requests run one at a time;
// overlapping polls are skipped.
import {loadChanges, readOpenDiff} from '../changes/changes.ts';
import {isDiffTab, tabPath} from '../editor/editor.ts';
import {panes} from '../editor/tab-layout.ts';
import type {Store} from '../store.ts';
import {loadFolder, readOpenFile} from './files.ts';
import type {SessionFiles} from './types.ts';

// shownFolders are the root and each expanded folder whose ancestors are all
// expanded.
function shownFolders(files: SessionFiles): string[] {
  const shown = (dir: string): boolean => dir === '' || (dir in files.expanded && shown(dir.slice(0, Math.max(0, dir.lastIndexOf('/')))));
  return ['', ...Object.keys(files.expanded).filter(shown)];
}

export function createFilePoller(store: Store) {
  let running = false;
  return async function poll() {
    const {visible, session, workbench} = store.get();
    if (running || !visible || !session) return;
    running = true;
    try {
      if (workbench.rightPanel && workbench.rightTab === 'files') {
        for (const dir of shownFolders(session.files)) await loadFolder(store, session.id, dir);
      }
      const shownPanes = panes(session.tabs.root);
      if ((workbench.rightPanel && workbench.rightTab === 'changes') || shownPanes.some(pane => isDiffTab(pane.active))) {
        await loadChanges(store);
      }
      for (const pane of shownPanes) {
        if (store.get().session?.id !== session.id) break;
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
