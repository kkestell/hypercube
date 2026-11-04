// While visible, refresh the shown Files or Changes list and each pane's
// active file or diff once a second. Requests run one at a time; overlapping
// polls are skipped.
import {loadChanges, readOpenDiff} from '../changes/changes.js';
import {tabPath} from '../editor/editor.js';
import {panes} from '../editor/tab-layout.js';
import {loadFolder, readOpenFile} from './files.js';

// shownFolders are the root and each expanded folder whose ancestors are all
// expanded.
function shownFolders(files) {
  const shown = dir => dir === '' || (files.expanded[dir] && shown(dir.slice(0, Math.max(0, dir.lastIndexOf('/')))));
  return ['', ...Object.keys(files.expanded).filter(shown)];
}

export function createFilePoller(store) {
  let running = false;
  return async function poll() {
    const {visible, session, workbench} = store.get();
    if (running || !visible || !session) return;
    running = true;
    try {
      if (workbench.rightPanel && workbench.rightTab === 'files') {
        for (const dir of shownFolders(session.files)) await loadFolder(store, session.id, dir);
      }
      if (workbench.rightPanel && workbench.rightTab === 'changes') await loadChanges(store);
      for (const pane of panes(session.tabs.root)) {
        if (store.get().session?.id !== session.id) break;
        if (pane.active.startsWith('diff:')) await readOpenDiff(store, tabPath(pane.active));
        if (pane.active.startsWith('file:')) await readOpenFile(store, tabPath(pane.active));
      }
    } finally {
      running = false;
    }
  };
}

export function startFilePolling(store) {
  setInterval(createFilePoller(store), 1000);
}
