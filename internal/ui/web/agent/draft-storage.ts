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

function readSaved(text: string): Pick<State, 'drafts' | 'unsent'> {
  const saved: unknown = JSON.parse(text);
  if (!isRecord(saved) || !isRecord(saved.drafts)) throw new Error('Invalid saved drafts.');
  const drafts: Record<string, Draft> = {};
  for (const [key, text] of Object.entries(saved.drafts)) {
    if (typeof text !== 'string') throw new Error('Invalid saved draft text.');
    drafts[key] = {text, images: [], reading: 0};
  }
  let unsent: Unsent | null = null;
  if (drafts[unsentKey]) {
    const context = saved.unsent;
    if (!isRecord(context) || !isNullableString(context.workspace) || !isNullableString(context.profile) || typeof context.worktree !== 'boolean') {
      throw new Error('Invalid new-session draft.');
    }
    unsent = {workspace: context.workspace, profile: context.profile, worktree: context.worktree, identity: {}, sending: false};
  }
  return {drafts, unsent};
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
      showError(store, 'Unsent text could not be saved and will be lost on reload: ' + reason(error));
    }
  });
}
