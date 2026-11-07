// Navigation preserves raw session IDs; workspace dashboards have their own key.
import type {State} from '../state.ts';
import {sessionOf} from '../sessions/lookup.ts';

export type EditorTarget = {session: string} | {workspace: string};
export const workspaceKey = (id: string) => 'workspace:' + id;
export const targetKey = (target: EditorTarget) => 'workspace' in target ? workspaceKey(target.workspace) : target.session;
export const selectionTarget = (selected: string | null): EditorTarget | null => !selected ? null
  : selected.startsWith('workspace:') ? {workspace: selected.slice(10)} : {session: selected};
export function selectedWorkspace(state: State) {
  const target = selectionTarget(state.selected);
  return target && ('workspace' in target ? state.workspaces.find(w => w.id === target.workspace) : sessionOf(state, target.session)?.workspace);
}
export function targetExists(state: State, target: EditorTarget) {
  return 'workspace' in target ? state.workspaces.some(w => w.id === target.workspace) : !!sessionOf(state, target.session);
}
export function editorDirectory(state: State) {
  const target = state.editor?.target;
  return target && ('workspace' in target ? state.workspaces.find(w => w.id === target.workspace)?.path : sessionOf(state, target.session)?.session.dir);
}
