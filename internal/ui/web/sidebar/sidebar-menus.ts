// The sidebar's menus: a session's, an unsent session's, and a workspace's.
import {discardUnsent, isUnsent} from '../agent/drafts.ts';
import {openItems} from '../app/desktop-actions.ts';
import {archiveSession, deleteSession, deleteWorkspace, restartSession, unarchiveSession} from '../sessions/session-actions.ts';
import type {Session, Workspace} from '../sessions/types.ts';
import type {ContextMenuItem} from '../shared/context-menu.tsx';
import type {Store} from '../store.ts';

export const sessionItems = (store: Store, session: Session, rename: () => void): ContextMenuItem[][] => {
  if (isUnsent(session.id)) return [[{label: 'Discard Draft', icon: 'trash', destructive: true, run: () => store.update(state => discardUnsent(state, session.id))}]];
  const renameItem: ContextMenuItem = {label: 'Rename', icon: 'edit', run: rename};
  const deleteItem: ContextMenuItem = {label: 'Delete Session', icon: 'trash', destructive: true, run: () => deleteSession(store, session)};
  if (!session.parent && session.ended) return [[{label: 'Unarchive', icon: 'archive', run: () => unarchiveSession(store, session.id)}, deleteItem]];
  const restartItem: ContextMenuItem = {label: 'Restart Agent', icon: 'refresh', run: () => restartSession(store, session.id)};
  if (session.parent) return [session.ended ? [renameItem] : [renameItem, restartItem], openItems(store, session.dir), session.ended ? [deleteItem] : []];
  return [
    [renameItem, restartItem],
    session.children.some(child => child.ended) ? [endedItem(store, session)] : [],
    openItems(store, session.dir),
    [{label: 'Archive Session', icon: 'archive', run: () => archiveSession(store, session.id)}, deleteItem],
  ];
};

// endedItem shows or hides an active parent's ended children in the sidebar.
const endedItem = (store: Store, session: Session): ContextMenuItem => {
  const shown = !!store.get().endedShown[session.id];
  const run = () => store.update(state => {
    const endedShown = {...state.endedShown};
    if (shown) delete endedShown[session.id];
    else endedShown[session.id] = true;
    return {...state, endedShown};
  });
  return shown
    ? {label: 'Hide Ended Sessions', icon: 'eye-off', run}
    : {label: 'Show Ended Sessions', icon: 'eye', run};
};

export const workspaceItems = (store: Store, workspace: Workspace): ContextMenuItem[][] => [
  openItems(store, workspace.path),
  [{label: 'Delete Workspace', icon: 'trash', destructive: true, run: () => deleteWorkspace(store, workspace)}],
];
