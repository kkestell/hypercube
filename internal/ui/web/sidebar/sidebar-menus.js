// The sidebar's menus: a session's, a workspace's, and the lead picker of a
// workspace's New Session button, which lists its lead profiles and opens an
// unsent session with the chosen one.
import {useCallback, useEffect, useRef, useState} from 'preact/hooks';
import {fetchLeads} from '../api.js';
import {deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.js';
import {failed, openUnsent} from '../sessions/selection.js';

const sessionItems = (store, session) => session.parent
  ? session.ended ? [{label: 'Delete Session', run: () => deleteSession(store, session)}] : []
  : [
  session.running && !failed(session)
    ? {label: 'Stop Session', run: () => stopSession(store, session.id)}
    : {label: 'Start Session', run: () => startSession(store, session.id)},
  {label: 'Delete Session', run: () => deleteSession(store, session)},
];

const workspaceItems = (store, workspace) => [{label: 'Delete Workspace', run: () => deleteWorkspace(store, workspace)}];

// useSidebarMenu holds the sidebar's open menu, {x, y, items} or null. The
// lead picker opens when its profiles arrive, unless another menu opened or
// closed meanwhile.
export function useSidebarMenu(store) {
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
  const showLeads = async (point, workspace) => {
    const asked = ++request.current;
    const leads = await fetchLeads(store);
    if (!leads || asked !== request.current) return;
    show(point, leads.map(profile => ({label: profile.description, run: () => store.update(state => openUnsent(state, workspace.id, profile))})));
  };
  return {
    menu,
    close,
    showSession: (point, session) => {
      const items = sessionItems(store, session);
      if (items.length) show(point, items);
    },
    showWorkspace: (point, workspace) => show(point, workspaceItems(store, workspace)),
    showLeads,
  };
}
