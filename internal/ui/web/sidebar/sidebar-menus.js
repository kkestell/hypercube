// The sidebar's menus: a session's and a workspace's.
import {post} from '../api.js';
import {deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.js';
import {failed} from '../sessions/selection.js';

const openItems = (store, kind, id) => [
  ['Open in VS Code', 'vscode'], ['Reveal in Finder', 'finder'], ['Open in Terminal', 'terminal'],
].map(([label, app]) => ({label, run: () => post(store, 'open', {kind, id, app})}));

// sessionItems is a session's menu. stoppedShown tells whether a parent shows
// its stopped children, and toggleStopped flips it.
export const sessionItems = (store, session, {stoppedShown, toggleStopped, rename}) => {
  const renameItem = {label: 'Rename', run: rename};
  const deleteItem = {label: 'Delete Session', run: () => deleteSession(store, session)};
  if (session.parent) return [renameItem, ...(session.ended ? [deleteItem] : []), ...openItems(store, 'session', session.id)];
  return [
    session.running && !failed(session)
      ? {label: 'Stop Session', run: () => stopSession(store, session.id)}
      : {label: 'Start Session', run: () => startSession(store, session.id)},
    ...session.children.length
      ? [{label: stoppedShown ? 'Hide Stopped Sessions' : 'Show Stopped Sessions', run: toggleStopped}]
      : [],
    renameItem,
    deleteItem,
    ...openItems(store, 'session', session.id),
  ];
};

export const workspaceItems = (store, workspace, stoppedShown, toggleStopped) => [
  {label: stoppedShown ? 'Hide Stopped Sessions' : 'Show Stopped Sessions', run: toggleStopped},
  {label: 'Delete Workspace', run: () => deleteWorkspace(store, workspace)},
  ...openItems(store, 'workspace', workspace.id),
];
