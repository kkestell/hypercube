// The sidebar's menus: a session's and a workspace's.
import {deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.js';
import {failed} from '../sessions/selection.js';

// sessionItems is a session's menu, which may be empty. stoppedShown tells
// whether a parent shows its stopped children, and toggleStopped flips it.
export const sessionItems = (store, session, stoppedShown, toggleStopped) => session.parent
  ? session.ended ? [{label: 'Delete Session', run: () => deleteSession(store, session)}] : []
  : [
  session.running && !failed(session)
    ? {label: 'Stop Session', run: () => stopSession(store, session.id)}
    : {label: 'Start Session', run: () => startSession(store, session.id)},
  ...session.children.length
    ? [{label: stoppedShown ? 'Hide Stopped Sessions' : 'Show Stopped Sessions', run: toggleStopped}]
    : [],
  {label: 'Delete Session', run: () => deleteSession(store, session)},
];

export const workspaceItems = (store, workspace) => [{label: 'Delete Workspace', run: () => deleteWorkspace(store, workspace)}];
