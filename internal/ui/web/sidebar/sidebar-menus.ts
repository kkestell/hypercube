// The sidebar's menus: a session's, an unsent session's, and a workspace's.
import {discardUnsent, isUnsent} from '../agent/drafts.ts';
import {post} from '../api.ts';
import {archiveSession, deleteSession, deleteWorkspace, unarchiveSession} from '../sessions/session-actions.ts';
import type {Session, Workspace} from '../sessions/types.ts';
import type {ContextMenuItem} from '../shared/context-menu.tsx';
import type {IconName} from '../shared/icon.tsx';
import type {Store} from '../store.ts';

const openApps: [string, string, IconName][] = [
  ['Open in VS Code', 'vscode', 'code'], ['Reveal in Finder', 'finder', 'folder-open'], ['Open in Terminal', 'terminal', 'terminal'],
];
const openItems = (store: Store, kind: 'session' | 'workspace', id: string): ContextMenuItem[] =>
  openApps.map(([label, app, icon]) => ({label, icon, run: () => post(store, 'open', {kind, id, app})}));

export const sessionItems = (store: Store, session: Session, rename: () => void): ContextMenuItem[][] => {
  if (isUnsent(session.id)) return [[{label: 'Discard Draft', icon: 'trash', destructive: true, run: () => store.update(state => discardUnsent(state, session.id))}]];
  const renameItem: ContextMenuItem = {label: 'Rename', icon: 'edit', run: rename};
  const deleteItem: ContextMenuItem = {label: 'Delete Session', icon: 'trash', destructive: true, run: () => deleteSession(store, session)};
  if (!session.parent && session.ended) return [[{label: 'Unarchive', icon: 'archive', run: () => unarchiveSession(store, session.id)}, deleteItem]];
  if (session.parent) return [[renameItem], openItems(store, 'session', session.id), session.ended ? [deleteItem] : []];
  return [
    [renameItem],
    openItems(store, 'session', session.id),
    [{label: 'Archive Session', icon: 'archive', run: () => archiveSession(store, session.id)}, deleteItem],
  ];
};

export const workspaceItems = (store: Store, workspace: Workspace): ContextMenuItem[][] => [
  openItems(store, 'workspace', workspace.id),
  [{label: 'Delete Workspace', icon: 'trash', destructive: true, run: () => deleteWorkspace(store, workspace)}],
];
