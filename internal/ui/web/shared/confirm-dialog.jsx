// VS Code's modal confirmation dialog: a warning, the question, a detail line,
// and Cancel beside the primary button, in macOS order. The page asks one
// question at a time, held in state.confirmation. The primary button takes the
// focus, so Enter confirms, and Escape cancels.
import {useEffect, useRef} from 'preact/hooks';
import {useStore} from '../store.js';
import './confirm-dialog.css';

// askToConfirm shows the dialog for {message, detail, primary, accept}, and
// runs accept if the user confirms.
export const askToConfirm = (store, confirmation) => store.update(state => ({...state, confirmation}));

export function ConfirmDialog() {
  const [state, store] = useStore();
  const {confirmation} = state;
  const dialog = useRef(null);

  useEffect(() => {
    if (confirmation) dialog.current.showModal();
  }, [confirmation]);

  if (!confirmation) return null;
  const close = () => store.update(s => ({...s, confirmation: null}));
  const accept = () => {
    close();
    confirmation.accept();
  };
  // Handling Escape here keeps the native dialog from closing itself.
  const key = e => {
    if (e.key !== 'Escape') return;
    e.preventDefault();
    close();
  };

  return (
    <dialog ref={dialog} class="confirm-dialog" onKeyDown={key}>
      <div class="confirm-dialog-toolbar">
        <vscode-toolbar-button icon="close" label="Close Dialog" title="Close Dialog" onClick={close} />
      </div>
      <div class="confirm-dialog-message-row">
        <vscode-icon name="warning" size={48} class="confirm-dialog-icon" />
        <div class="confirm-dialog-text">
          <div class="confirm-dialog-message">{confirmation.message}</div>
          <div class="confirm-dialog-detail">{confirmation.detail}</div>
        </div>
      </div>
      <div class="confirm-dialog-buttons">
        <vscode-button secondary onClick={close}>Cancel</vscode-button>
        <vscode-button autofocus onClick={accept}>{confirmation.primary}</vscode-button>
      </div>
    </dialog>
  );
}
