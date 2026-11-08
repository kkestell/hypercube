// While visible, refresh the shown Files tree or Git changes, and each
// pane's active file or diff, once a second. The chat's branch bar and active
// diffs also refresh changes. Requests run one at a
// time; overlapping polls are skipped.
import {loadChanges, readOpenDiff} from '../changes/changes.ts';
import {isDiffTab, tabPath} from '../editor/editor.ts';
import {panes} from '../editor/tab-layout.ts';
import type {Store} from '../store.ts';
import {loadTree, readOpenFile, shownFolders} from './files.ts';

export function createFilePoller(store: Store) {
  let running = false;
  return async function poll() {
    const {visible, editor, editorVersion, workbench} = store.get();
    if (running || !visible || !editor) return;
    running = true;
    try {
      const tree = workbench.rightPanel && workbench.rightTab === 'files';
      const git = workbench.rightPanel && workbench.rightTab === 'git';
      const chat = !!store.get().session && !store.get().unsent;
      if (tree) {
        await loadTree(store, shownFolders(editor.files));
      }
      const shownPanes = panes(editor.tabs.root);
      const left = () => store.get().editor?.id !== editor.id || store.get().editorVersion !== editorVersion;
      if (!left() && (chat || tree || git || shownPanes.some(pane => isDiffTab(pane.active)))) await loadChanges(store);
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
