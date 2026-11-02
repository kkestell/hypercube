// The sidebar's menus: a session's, a workspace's, and the lead picker of a
// workspace's New Session button, which lists its lead profiles and opens an
// unsent session with the chosen one.
import {useCallback, useRef, useState} from 'preact/hooks';
import {fetchLeads} from '../api.js';
import {deleteSession, deleteWorkspace, startSession, stopSession} from '../sessions/session-actions.js';
import {failed, openUnsent} from '../sessions/selection.js';

const sessionItems = session => [
  session.running && !failed(session)
    ? {label: 'Stop Session', run: () => stopSession(session.id)}
    : {label: 'Start Session', run: () => startSession(session.id)},
  {label: 'Delete Session', run: () => deleteSession(session)},
];

const workspaceItems = workspace => [{label: 'Delete Workspace', run: () => deleteWorkspace(workspace)}];

// useSidebarMenu holds the sidebar's open menu, {x, y, items} or null. The
// lead picker opens when its profiles arrive, unless another menu opened or
// closed meanwhile.
export function useSidebarMenu(store) {
  const [menu, setMenu] = useState(null);
  const request = useRef(0);
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
    const leads = await fetchLeads();
    if (!leads || asked !== request.current) return;
    show(point, leads.map(profile => ({label: profile.description, run: () => store.update(state => openUnsent(state, workspace.id, profile))})));
  };
  return {
    menu,
    close,
    showSession: (point, session) => show(point, sessionItems(session)),
    showWorkspace: (point, workspace) => show(point, workspaceItems(workspace)),
    showLeads,
  };
}
