// While the page is visible, the Files tree and open files refresh once a
// second from the store's state: the listed folders the tree shows, when the
// right panel shows Files, and the active file tab of each pane. Requests go
// one at a time, and a poll is skipped while the previous one runs.
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
      for (const pane of panes(session.tabs.root)) {
        const path = tabPath(pane.active);
        if (path !== null) await readOpenFile(store, path);
      }
    } finally {
      running = false;
    }
  };
}

export function startFilePolling(store) {
  setInterval(createFilePoller(store), 1000);
}
