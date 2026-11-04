// The sidebar's menus: a session's and a workspace's.
import {post} from '../api.ts';
import {archiveSession, deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.ts';
import {failed} from '../sessions/selection.ts';
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
  const renameItem: ContextMenuItem = {label: 'Rename', icon: 'edit', run: rename};
  const deleteItem: ContextMenuItem = {label: 'Delete Session', icon: 'trash', destructive: true, run: () => deleteSession(store, session)};
  if (!session.parent && session.ended) return [[deleteItem]];
  if (session.parent) return [[renameItem], openItems(store, 'session', session.id), session.ended ? [deleteItem] : []];
  const running = session.running && !failed(session);
  return [
    [
      running
        ? {label: 'Stop Session', icon: 'stop', run: () => stopSession(store, session.id)}
        : {label: 'Start Session', icon: 'play', run: () => startSession(store, session.id)},
      renameItem,
    ],
    openItems(store, 'session', session.id),
    [{label: 'Archive Session', icon: 'archive', run: () => archiveSession(store, session.id)}, deleteItem],
  ];
};

export const workspaceItems = (store: Store, workspace: Workspace): ContextMenuItem[][] => [
  openItems(store, 'workspace', workspace.id),
  [{label: 'Delete Workspace', icon: 'trash', destructive: true, run: () => deleteWorkspace(store, workspace)}],
];
