// Persist view choices, refreshing transcript and file data on every attachment.
import {showError} from '../notifications/errors.ts';
import {sessionView} from '../sessions/view-state.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {readSavedViews, type SavedViews} from './saved-views.ts';

const storageKey = 'hypercube.views';
const reason = (error: unknown) => error instanceof Error ? error.message : String(error);

function snapshot(state: State): SavedViews {
  const sessionViews = {...state.sessionViews};
  if (state.session) sessionViews[state.session.id] = sessionView(state.session);
  return {selected: state.selected, workbench: state.workbench, sessionViews};
}

function viewChanged(before: State, after: State) {
  if (before.selected !== after.selected || before.workbench !== after.workbench || before.sessionViews !== after.sessionViews) return true;
  const a = before.session, b = after.session;
  return a?.id !== b?.id || a?.tabs !== b?.tabs || a?.activePane !== b?.activePane || a?.panel !== b?.panel || a?.expanded !== b?.expanded
    || a?.scroll !== b?.scroll || a?.files.expanded !== b?.files.expanded || a?.files.open !== b?.files.open;
}

export function startViewStorage(store: Store): () => void {
  let written: string | null = null;
  let legacy = false;
  try {
    written = localStorage.getItem(storageKey);
    if (written !== null) {
      const saved = readSavedViews(written);
      store.update(state => ({...state, workbench: saved.workbench, sessionViews: saved.sessionViews, selected: state.selected || saved.selected}));
    } else {
      const wrap = localStorage.getItem('hypercube.wrap');
      const fixedWidth = localStorage.getItem('hypercube.fixedWidth');
      legacy = wrap !== null || fixedWidth !== null;
      store.update(state => ({...state, workbench: {...state.workbench, wrap: wrap === 'true', fixedWidth: fixedWidth !== 'false'}}));
    }
  } catch (error) {
    showError(store, 'Could not restore view state: ' + reason(error));
  }

  let previous = store.get();
  let failed = false;
  const save = (state: State) => {
    const text = JSON.stringify(snapshot(state));
    if (text === written && !legacy) return;
    try {
      localStorage.setItem(storageKey, text);
      written = text;
      if (legacy) {
        localStorage.removeItem('hypercube.wrap');
        localStorage.removeItem('hypercube.fixedWidth');
        legacy = false;
      }
      failed = false;
    } catch (error) {
      if (failed) return;
      failed = true;
      showError(store, 'View state could not be saved: ' + reason(error));
    }
  };
  if (legacy) save(previous);
  return store.subscribe(state => {
    const changed = viewChanged(previous, state);
    previous = state;
    if (changed) save(state);
  });
}
