// Save typed text on each draft change so reloads and app restarts keep it.
// Image attachments and in-progress reads remain in memory.
import {showError} from '../notifications/errors.ts';
import type {NewBranch, Unsent} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {unsentKey, withDraftDefaults} from './drafts.ts';
import type {Draft, Settings} from './types.ts';

const storageKey = 'hypercube.drafts';

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const isNullableString = (value: unknown): value is string | null => value === null || typeof value === 'string';

const isSettings = (value: unknown): value is Settings => isRecord(value) &&
  typeof value.harness === 'string' && typeof value.model === 'string' && typeof value.effort === 'string';

const isBranch = (value: unknown): value is NewBranch => isRecord(value) && typeof value.name === 'string' && typeof value.start === 'string';

// Settings and worktree are null until the defaults arrive or the user chooses.
// Drafts saved before branch choices existed have no branch.
function readContext(value: unknown): Unsent {
  if (!isRecord(value) || !isNullableString(value.workspace) || (value.worktree !== null && typeof value.worktree !== 'boolean') ||
    (value.settings !== null && !isSettings(value.settings)) || (value.branch != null && !isBranch(value.branch))) {
    throw new Error('Invalid new-session draft.');
  }
  return {workspace: value.workspace, settings: value.settings, worktree: value.worktree, branch: value.branch ?? null, identity: {}, sending: false};
}

function readSaved(text: string): Pick<State, 'drafts' | 'unsent' | 'unsentDrafts'> {
  const saved: unknown = JSON.parse(text);
  if (!isRecord(saved) || !isRecord(saved.drafts)) throw new Error('Invalid saved drafts.');
  if (!isRecord(saved.unsentDrafts)) throw new Error('Invalid new-session drafts.');
  const drafts: Record<string, Draft> = {};
  for (const [key, text] of Object.entries(saved.drafts)) {
    if (typeof text !== 'string') throw new Error('Invalid saved draft text.');
    drafts[key] = {text, images: [], reading: 0};
  }
  let unsent = saved.unsent === null ? null : readContext(saved.unsent);
  if (unsent && !drafts[unsentKey(unsent.workspace)]) unsent = null;
  const unsentDrafts: Record<string, Unsent> = {};
  for (const [key, value] of Object.entries(saved.unsentDrafts)) {
    const context = readContext(value);
    if (key !== unsentKey(context.workspace)) throw new Error('Invalid new-session draft key.');
    if (drafts[key]) unsentDrafts[key] = context;
  }
  return {drafts, unsent, unsentDrafts};
}

const reason = (error: unknown) => error instanceof Error ? error.message : String(error);

export function startDraftStorage(store: Store): () => void {
  try {
    const text = localStorage.getItem(storageKey);
    if (text !== null) {
      const saved = readSaved(text);
      store.update(state => withDraftDefaults({...state, ...saved, selected: saved.unsent ? null : state.selected}));
    }
  } catch (error) {
    showError(store, 'Could not restore unsent text: ' + reason(error));
  }

  let previous = store.get();
  let saveFailed = false;
  return store.subscribe(state => {
    const changed = state.drafts !== previous.drafts || state.unsent !== previous.unsent || state.unsentDrafts !== previous.unsentDrafts;
    previous = state;
    if (!changed) return;
    const drafts = Object.fromEntries(Object.entries(state.drafts).filter(([, draft]) => draft.text).map(([key, draft]) => [key, draft.text]));
    const context = (draft: Unsent) => ({workspace: draft.workspace, settings: draft.settings, worktree: draft.worktree, branch: draft.branch});
    const unsent = state.unsent && drafts[unsentKey(state.unsent.workspace)]
      ? context(state.unsent)
      : null;
    const unsentDrafts = Object.fromEntries(Object.entries(state.unsentDrafts).filter(([key]) => drafts[key]).map(([key, draft]) => [key, context(draft)]));
    try {
      if (Object.keys(drafts).length) localStorage.setItem(storageKey, JSON.stringify({drafts, unsent, unsentDrafts}));
      else localStorage.removeItem(storageKey);
      saveFailed = false;
    } catch (error) {
      if (saveFailed) return;
      saveFailed = true;
      showError(store, 'Unsent text could not be saved and will be lost on reload: ' + reason(error));
    }
  });
}
