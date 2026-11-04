// Save typed text on each draft change so reloads and app restarts keep it.
// Image attachments and in-progress reads remain in memory.
import {showError} from '../notifications/errors.js';
import {unsentKey} from './drafts.js';

const storageKey = 'hypercube.drafts';

function readSaved(text) {
  const saved = JSON.parse(text);
  if (!saved || typeof saved.drafts !== 'object' || saved.drafts === null || Array.isArray(saved.drafts)) throw new Error('Invalid saved drafts.');
  const drafts = Object.fromEntries(Object.entries(saved.drafts).map(([key, text]) => {
    if (typeof text !== 'string') throw new Error('Invalid saved draft text.');
    return [key, {text, images: [], reading: 0}];
  }));
  let unsent = null;
  if (drafts[unsentKey]) {
    const context = saved.unsent;
    if (!context || (context.workspace !== null && typeof context.workspace !== 'string') ||
        (context.profile !== null && typeof context.profile !== 'string') || typeof context.worktree !== 'boolean') throw new Error('Invalid new-session draft.');
    unsent = {workspace: context.workspace, profile: context.profile, worktree: context.worktree, identity: {}, sending: false};
  }
  return {drafts, unsent};
}

export function startDraftStorage(store) {
  try {
    const text = localStorage.getItem(storageKey);
    if (text !== null) {
      const saved = readSaved(text);
      store.update(state => ({...state, ...saved, selected: saved.unsent ? null : state.selected}));
    }
  } catch (error) {
    showError(store, 'Could not restore unsent text: ' + error.message);
  }

  let previous = store.get();
  let saveFailed = false;
  return store.subscribe(state => {
    const changed = state.drafts !== previous.drafts || state.unsent !== previous.unsent;
    previous = state;
    if (!changed) return;
    const drafts = Object.fromEntries(Object.entries(state.drafts).filter(([, draft]) => draft.text).map(([key, draft]) => [key, draft.text]));
    const unsent = drafts[unsentKey] && state.unsent
      ? {workspace: state.unsent.workspace, profile: state.unsent.profile, worktree: state.unsent.worktree}
      : null;
    try {
      if (Object.keys(drafts).length) localStorage.setItem(storageKey, JSON.stringify({drafts, unsent}));
      else localStorage.removeItem(storageKey);
      saveFailed = false;
    } catch (error) {
      if (saveFailed) return;
      saveFailed = true;
      showError(store, 'Unsent text could not be saved and will be lost on reload: ' + error.message);
    }
  });
}
