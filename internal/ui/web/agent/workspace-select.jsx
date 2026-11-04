// Workspace selection below the new session's composer.
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
    </div>
  );
}
