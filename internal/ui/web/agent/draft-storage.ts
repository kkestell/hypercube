// Save typed text on each draft change so reloads and app restarts keep it.
// Image attachments and in-progress reads remain in memory.
import {showError} from '../notifications/errors.ts';
import type {Unsent} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {unsentKey} from './drafts.ts';
import type {Draft} from './types.ts';

const storageKey = 'hypercube.drafts';

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const isNullableString = (value: unknown): value is string | null => value === null || typeof value === 'string';

function readContext(value: unknown): Unsent {
  if (!isRecord(value) || !isNullableString(value.workspace) || !isNullableString(value.profile) || (value.worktree !== null && typeof value.worktree !== 'boolean')) {
    throw new Error('Invalid new-session draft.');
  }
  return {workspace: value.workspace, profile: value.profile, worktree: value.worktree, identity: {}, sending: false};
}

function readSaved(text: string): Pick<State, 'drafts' | 'unsent' | 'unsentDrafts'> {
  const saved: unknown = JSON.parse(text);
  if (!isRecord(saved) || !isRecord(saved.drafts)) throw new Error('Invalid saved drafts.');
  const drafts: Record<string, Draft> = {};
  for (const [key, text] of Object.entries(saved.drafts)) {
    if (typeof text !== 'string') throw new Error('Invalid saved draft text.');
    drafts[key] = {text, images: [], reading: 0};
  }
  let unsent: Unsent | null = null;
  const unsentDrafts: Record<string, Unsent> = {};
  if (drafts.new && !saved.unsent) throw new Error('Invalid new-session draft.');
  if (saved.unsent) {
    unsent = readContext(saved.unsent);
    // Previous versions stored the one new-session draft under "new".
    if (drafts.new) {
      drafts[unsentKey(unsent.workspace)] = drafts.new;
      delete drafts.new;
    }
    if (!drafts[unsentKey(unsent.workspace)]) unsent = null;
  }
  if (saved.unsentDrafts !== undefined) {
    if (!isRecord(saved.unsentDrafts)) throw new Error('Invalid new-session drafts.');
    for (const [key, value] of Object.entries(saved.unsentDrafts)) {
      const context = readContext(value);
      if (key !== unsentKey(context.workspace)) throw new Error('Invalid new-session draft key.');
      if (drafts[key]) unsentDrafts[key] = context;
    }
  }
  return {drafts, unsent, unsentDrafts};
}

const reason = (error: unknown) => error instanceof Error ? error.message : String(error);

export function startDraftStorage(store: Store): () => void {
  try {
    const text = localStorage.getItem(storageKey);
    if (text !== null) {
      const saved = readSaved(text);
      store.update(state => ({...state, ...saved, selected: saved.unsent ? null : state.selected}));
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
    const context = (draft: Unsent) => ({workspace: draft.workspace, profile: draft.profile, worktree: draft.worktree});
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
