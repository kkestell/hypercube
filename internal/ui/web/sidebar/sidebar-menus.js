// The sidebar's menus: a session's and a workspace's.
import {useCallback, useState} from 'preact/hooks';
import {deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.js';
import {failed} from '../sessions/selection.js';

const sessionItems = (store, session, showStopped, toggleStopped) => session.parent
  ? session.ended ? [{label: 'Delete Session', run: () => deleteSession(store, session)}] : []
  : [
  session.running && !failed(session)
    ? {label: 'Stop Session', run: () => stopSession(store, session.id)}
    : {label: 'Start Session', run: () => startSession(store, session.id)},
  ...session.children.length
    ? [{label: showStopped[session.id] ? 'Hide Stopped Sessions' : 'Show Stopped Sessions', run: () => toggleStopped(session.id)}]
    : [],
  {label: 'Delete Session', run: () => deleteSession(store, session)},
];

const workspaceItems = (store, workspace) => [{label: 'Delete Workspace', run: () => deleteWorkspace(store, workspace)}];

// useSidebarMenu holds one context menu, or null. showStopped maps a parent's
// ID to whether it shows its stopped nested sessions, and toggleStopped flips it.
export function useSidebarMenu(store, showStopped, toggleStopped) {
  const [menu, setMenu] = useState(null);

  const show = (point, items) => setMenu({...point, items});
  const close = useCallback(() => setMenu(null), []);
  return {
    menu,
    close,
    showSession: (point, session) => {
      const items = sessionItems(store, session, showStopped, toggleStopped);
      if (items.length) show(point, items);
    },
    hasSessionMenu: session => sessionItems(store, session, showStopped, toggleStopped).length > 0,
    showWorkspace: (point, workspace) => show(point, workspaceItems(store, workspace)),
  };
}
