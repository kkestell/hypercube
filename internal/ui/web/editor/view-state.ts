// Saved editor choices contain no server data or live handles.
import {panes, findPane} from './tab-layout.ts';
import type {EditorView, EditorViewState} from './types.ts';
import {unreadFile} from '../files/files.ts';
import {isDiffTab} from '../changes/changes.ts';

export function editorView(view: EditorView): EditorViewState {
  return {
    tabs: {...view.tabs, dragging: null}, activePane: view.activePane, panel: view.panel,
    scroll: view.scroll, folders: view.files.expanded,
    files: Object.fromEntries(Object.entries(view.files.open).map(([path, file]) => [path, {source: file.source, lines: file.lines}])),
  };
}
export function restoreView(editor: EditorView, view: EditorViewState): EditorView {
  const diffIds = panes(view.tabs.root).flatMap(pane => pane.tabs.filter(isDiffTab));
  return {...editor, tabs: view.tabs, activePane: view.activePane, panel: view.panel, scroll: view.scroll,
    files: {...editor.files, expanded: view.folders, open: Object.fromEntries(Object.entries(view.files).map(([path, file]) => [path, {...unreadFile, ...file}]))},
    diffs: Object.fromEntries(diffIds.map(id => [id, {text: null, message: null}])),
  };
}
// Late unmounts may save a departing view, but closed tabs stay forgotten.
export const ownsScroll = (view: Pick<EditorViewState, 'tabs'>, key: string, id: string) =>
  id === key || id === 'files-tree' || !!findPane(view.tabs.root, id);
