// Persist view choices, refreshing transcript and file data on every attachment.
import {showError} from '../notifications/errors.ts';
import {errorText} from '../shared/error-text.ts';
import {editorView} from '../editor/view-state.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {readSavedViews, type SavedViews} from './saved-views.ts';

const storageKey = 'hypercube.views';

function snapshot(state: State): SavedViews {
  const sessionViews = {...state.sessionViews};
  const workspaceViews = {...state.workspaceViews};
  if (state.editor) {
    const view = editorView(state.editor);
    const target = state.editor.target;
    if ('workspace' in target) workspaceViews[target.workspace] = view;
    else sessionViews[target.session] = {...view, expanded: state.session!.expanded};
  }
  return {selected: state.selected, workbench: state.workbench, sessionViews, workspaceViews};
}

function viewChanged(before: State, after: State) {
  if (before.selected !== after.selected || before.workbench !== after.workbench || before.sessionViews !== after.sessionViews || before.workspaceViews !== after.workspaceViews || before.session?.expanded !== after.session?.expanded) return true;
  const a = before.editor, b = after.editor;
  return a?.id !== b?.id || a?.tabs !== b?.tabs || a?.activePane !== b?.activePane || a?.panel !== b?.panel
    || a?.scroll !== b?.scroll || a?.files.expanded !== b?.files.expanded || a?.files.open !== b?.files.open;
}

export function startViewStorage(store: Store): () => void {
  let written: string | null = null;
  try {
    written = localStorage.getItem(storageKey);
    if (written !== null) {
      const saved = readSavedViews(written);
      store.update(state => ({...state, workbench: saved.workbench, sessionViews: saved.sessionViews, workspaceViews: saved.workspaceViews, selected: state.selected || saved.selected}));
    }
  } catch (error) {
    showError(store, 'Could not restore view state: ' + errorText(error));
  }

  let previous = store.get();
  let failed = false;
  const save = (state: State) => {
    const text = JSON.stringify(snapshot(state));
    if (text === written) return;
    try {
      localStorage.setItem(storageKey, text);
      written = text;
      failed = false;
    } catch (error) {
      if (failed) return;
      failed = true;
      showError(store, 'View state could not be saved: ' + errorText(error));
    }
  };
  return store.subscribe(state => {
    const changed = viewChanged(previous, state);
    previous = state;
    if (changed) save(state);
  });
}
