// The sidebar's rows for its selected tab, derived from the state: each
// workspace's top-level sessions, the unsent session first in the sessions
// tab, and each parent's listed children. An active parent lists its ended
// children only when its menu shows them, and always lists the shown session.
// While a title is renamed, the top-level sessions keep the order they had
// when renaming started.
import {shownId, unsentRow} from '../sessions/selection.ts';
import type {Session, Workspace} from '../sessions/types.ts';
import type {State} from '../state.ts';

// SidebarRow is one tree row. A row carries its level and its place among its
// siblings for assistive technology.
export interface SidebarRow {
  id: string;
  session: Session;
  level: 1 | 2;
  position: number;
  siblings: number;
  hasChildren: boolean;
  expanded: boolean;
  parent?: string;
}

// SessionGroup is a top-level row and its children's rows, which stay listed
// while the parent is collapsed so they can slide closed.
export interface SessionGroup {
  parent: SidebarRow;
  children: SidebarRow[];
}

export interface SidebarWorkspace {
  workspace: Workspace;
  groups: SessionGroup[];
}

function topSessions(state: State, workspace: Workspace): Session[] {
  const archived = state.workbench.sidebarTab === 'archived';
  const sessions = workspace.sessions.filter(top => top.ended === archived);
  const unsent = archived ? null : unsentRow(state, workspace.id);
  return unsent ? [unsent, ...sessions] : sessions;
}

export function sidebarWorkspaces(state: State): SidebarWorkspace[] {
  const shown = shownId(state);
  const collapsed = state.workbench.collapsedSessions;
  const listed = (parent: Session) => parent.ended || state.endedShown[parent.id]
    ? parent.children
    : parent.children.filter(child => !child.ended || child.id === shown);
  return state.workspaces.map(workspace => {
    const live = topSessions(state, workspace);
    const order = state.renaming?.order[workspace.id];
    const sessions = order ? order.flatMap(id => live.find(session => session.id === id) ?? []) : live;
    const groups = sessions.map((session, index): SessionGroup => {
      const children = listed(session);
      const expanded = !collapsed[session.id];
      return {
        parent: {id: session.id, session, level: 1, position: index + 1, siblings: sessions.length, hasChildren: children.length > 0, expanded},
        children: children.map((child, i) => ({id: child.id, session: child, level: 2, position: i + 1, siblings: children.length, hasChildren: false, expanded: false, parent: session.id})),
      };
    });
    return {workspace, groups};
  });
}

// groupRows are the rows of groups the keyboard reaches: each parent, and the
// children of an expanded one.
export const groupRows = (groups: SessionGroup[]) => groups.flatMap(({parent, children}) => [parent, ...parent.expanded ? children : []]);

// sidebarRows are the rows the keyboard reaches in the whole sidebar, in
// order, skipping collapsed workspaces.
export const sidebarRows = (state: State): SidebarRow[] => sidebarWorkspaces(state)
  .filter(({workspace}) => !state.workbench.collapsedWorkspaces[workspace.id])
  .flatMap(({groups}) => groupRows(groups));

// sidebarSession finds a listed row's session, including an unsent session.
export const sidebarSession = (state: State, id: string | undefined) => sidebarWorkspaces(state)
  .flatMap(({groups}) => groups.flatMap(({parent, children}) => [parent, ...children]))
  .find(row => row.id === id)?.session;

export function startRenaming(state: State, session: string): State {
  const order = Object.fromEntries(state.workspaces.map(workspace => [workspace.id, topSessions(state, workspace).map(top => top.id)]));
  return {...state, renaming: {session, order}};
}

export const stopRenaming = (state: State): State => state.renaming ? {...state, renaming: null} : state;
