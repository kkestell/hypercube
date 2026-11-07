import type {State} from '../state.ts';
import type {Session, Workspace} from './types.ts';

export const allSessions = (state: State): Session[] => state.workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children]));
// sessionOf finds a session, its workspace, and its parent when it is a child.
export function sessionOf(state: State, id: string | null): {workspace: Workspace; session: Session; parent: Session | null} | undefined {
  for (const workspace of state.workspaces) {
    for (const top of workspace.sessions) {
      if (top.id === id) return {workspace, session: top, parent: null};
      const child = top.children.find(s => s.id === id);
      if (child) return {workspace, session: child, parent: top};
    }
  }
}
