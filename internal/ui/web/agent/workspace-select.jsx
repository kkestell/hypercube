// Workspace selection below the new session's composer. Adding a workspace
// selects it without replacing the draft or profile.
import {addWorkspace} from '../sessions/session-actions.js';
import {unsentWorkspace} from '../sessions/selection.js';
import {useStore} from '../store.js';
import './workspace-select.css';

export function WorkspaceSelect() {
  const [state, store] = useStore();
  const {unsent, workspaces} = state;
  const choose = e => {
    const workspace = workspaces.find(w => w.id === e.currentTarget.value);
    if (workspace) store.update(s => unsentWorkspace(s, workspace.id));
  };
  const add = async () => {
    const workspace = await addWorkspace(store);
    if (workspace && store.get().unsent?.identity === unsent.identity) {
      store.update(s => unsentWorkspace(s, workspace));
    }
  };

  return (
    <div class="workspace-select">
      <vscode-single-select class="workspace-select-control" label="Workspace" position="above" disabled={unsent.sending} onChange={choose}>
        {!unsent.workspace && <vscode-option value="" selected disabled>Select a workspace</vscode-option>}
        {workspaces.map(workspace => (
          <vscode-option key={workspace.id} value={workspace.id} description={workspace.path} selected={workspace.id === unsent.workspace}>
            {workspace.name}
          </vscode-option>
        ))}
      </vscode-single-select>
      <vscode-button secondary icon="add" disabled={unsent.sending} onClick={add}>Add Workspace</vscode-button>
    </div>
  );
}
