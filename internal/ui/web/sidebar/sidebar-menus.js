// The sidebar's menus: a session's, a workspace's, and the profile picker of a
// workspace's New Session button, which lists its profiles and opens an
// unsent session with the chosen one.
import {useCallback, useEffect, useRef, useState} from 'preact/hooks';
import {fetchProfiles} from '../api.js';
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

// useSidebarMenu holds one context menu or profile picker, or null.
// showStopped maps a parent's ID to whether it shows its stopped nested
// sessions, and toggleStopped flips it. The profile picker opens when its
// profiles arrive, unless another menu opened or closed meanwhile.
export function useSidebarMenu(store, showStopped, toggleStopped) {
  const [menu, setMenu] = useState(null);
  const request = useRef(0);

  // The picker can be cancelled before a fetched menu has mounted.
  useEffect(() => {
    const cancelPending = e => {
      if (e.type !== 'keydown' || e.key === 'Escape') request.current++;
    };
    const events = ['pointerdown', 'keydown', 'blur'];
    for (const event of events) addEventListener(event, cancelPending, event === 'pointerdown');
    return () => {
      request.current++;
      for (const event of events) removeEventListener(event, cancelPending, event === 'pointerdown');
    };
  }, []);

  const show = (point, items) => {
    request.current++;
    setMenu({...point, items});
  };
  const close = useCallback(() => {
    request.current++;
    setMenu(null);
  }, []);
  const showProfiles = async (point, workspace) => {
    const asked = ++request.current;
    const profiles = await fetchProfiles(store);
    if (!profiles || asked !== request.current) return;
    setMenu({...point, profiles, workspace});
  };
  return {
    menu,
    close,
    showSession: (point, session) => {
      const items = sessionItems(store, session, showStopped, toggleStopped);
      if (items.length) show(point, items);
    },
    hasSessionMenu: session => sessionItems(store, session, showStopped, toggleStopped).length > 0,
    showWorkspace: (point, workspace) => show(point, workspaceItems(store, workspace)),
    showProfiles,
  };
}
