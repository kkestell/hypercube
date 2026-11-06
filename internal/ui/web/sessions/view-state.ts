// A session keeps its view choices while transcript and file reads are refreshed.
import {panes, findPane} from '../editor/tab-layout.ts';
import {unreadFile} from '../files/files.ts';
import type {AttachedSession, SessionViewState} from './types.ts';

export function sessionView(session: AttachedSession): SessionViewState {
  return {
    tabs: {...session.tabs, dragging: null},
    activePane: session.activePane,
    expanded: session.expanded,
    scroll: session.scroll,
    folders: session.files.expanded,
    files: Object.fromEntries(Object.entries(session.files.open).map(([path, file]) => [path, {source: file.source, lines: file.lines}])),
  };
}

export function restoreView(session: AttachedSession, view: SessionViewState): AttachedSession {
  const diffPaths = panes(view.tabs.root).flatMap(pane => pane.tabs.filter(id => id.startsWith('diff:')).map(id => id.slice(5)));
  return {
    ...session,
    tabs: view.tabs,
    activePane: view.activePane,
    expanded: view.expanded,
    scroll: view.scroll,
    files: {listings: {}, expanded: view.folders, open: Object.fromEntries(Object.entries(view.files).map(([path, file]) => [path, {...unreadFile, ...file}]))},
    diffs: Object.fromEntries(diffPaths.map(path => [path, {text: null, message: null}])),
  };
}

// Late unmounts may save a departing session, but closed tabs must stay forgotten.
export const ownsScroll = (view: Pick<SessionViewState, 'tabs'>, session: string, id: string) =>
  id === session || id === 'files-tree' || !!findPane(view.tabs.root, id);
