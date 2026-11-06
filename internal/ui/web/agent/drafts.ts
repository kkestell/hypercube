// What the user has typed and attached but not yet sent, by draft key: an
// session ID for its composer, a workspace's unsentKey for a new session, or
// '<session>/<request>' for a typed answer to an attention request. Drafts
// outlive their views, so text survives session switches and closed tabs.
import {imageBytes, type Image, imageTypes, maxImageBytes, readImage} from '../images.ts';
import {showError} from '../notifications/errors.ts';
import type {Session, Unsent} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {Draft, Settings} from './types.ts';

// Each workspace has one unfinished new session, with a stable draft key.
export const unsentKey = (workspace: string | null) => 'new/' + (workspace ?? '');
export const isUnsent = (key: string) => key.startsWith('new/');
export const unsentOf = (state: State, key: string): Unsent | undefined =>
  state.unsent && unsentKey(state.unsent.workspace) === key ? state.unsent : state.unsentDrafts[key];

const emptyDraft: Draft = {text: '', images: [], reading: 0};

export const draftOf = (state: State, key: string): Draft => state.drafts[key] ?? emptyDraft;

// A prompt has text, images, or both, and waits for images still being read.
const hasContent = (draft: Draft) => !draft.reading && (!!draft.text.trim() || draft.images.length > 0);

// draftDefaults are a new session's first settings: those of the newest
// top-level session in its workspace, or the configured default.
export function draftDefaults(state: State, workspace: string | null): Settings | null {
  const newest = state.workspaces.find(w => w.id === workspace)?.sessions[0];
  if (newest) return {harness: newest.harness, model: newest.model, effort: newest.effort};
  return state.config?.default_selection ?? null;
}

// withDraftDefaults gives new-session drafts without settings their defaults
// once the workspaces or config that supply them arrive. Settings a draft
// already has never change by themselves.
export function withDraftDefaults(state: State): State {
  const fill = (draft: Unsent) => {
    if (draft.settings || draft.sending) return draft;
    const settings = draftDefaults(state, draft.workspace);
    return settings ? {...draft, settings} : draft;
  };
  const unsent = state.unsent && fill(state.unsent);
  const filled = Object.entries(state.unsentDrafts).map(([key, draft]) => [key, fill(draft)] as const);
  if (unsent === state.unsent && filled.every(([key, draft]) => draft === state.unsentDrafts[key])) return state;
  return {...state, unsent, unsentDrafts: Object.fromEntries(filled)};
}

// A new session needs a workspace and settings before its first prompt can send.
const unsentReady = (state: State, key: string) => {
  const unsent = unsentOf(state, key);
  return !!unsent?.workspace && !!unsent.settings && !unsent.sending;
};

export const canSend = (state: State, session: Pick<Session, 'id' | 'ended' | 'activity'>, text = draftOf(state, session.id).text) =>
  !session.ended && hasContent({...draftOf(state, session.id), text}) && (!isUnsent(session.id) || unsentReady(state, session.id));

function withDraft(state: State, key: string, change: (draft: Draft) => Draft): State {
  const draft = change(draftOf(state, key));
  const drafts = {...state.drafts};
  if (draft.text || draft.images.length || draft.reading || draft.choices?.length) drafts[key] = draft;
  else delete drafts[key];
  return {...state, drafts};
}

// cleared empties a draft, as after its prompt is sent.
export const cleared = (state: State, key: string): State => withDraft(state, key, () => emptyDraft);

export const typed = (state: State, key: string, text: string): State => withDraft(state, key, draft => ({...draft, text}));

export const selectedChoices = (state: State, key: string, choices: string[]): State => withDraft(state, key, draft => ({...draft, choices}));

export const removeImage = (state: State, key: string, image: Image): State => withDraft(state, key, draft => ({...draft, images: draft.images.filter(i => i !== image)}));

// restored puts a refused prompt back, unless the user has started another.
export const restored = (state: State, key: string, prompt: Pick<Draft, 'text' | 'images'>): State => withDraft(state, key, draft => ({
  ...draft,
  text: draft.text || prompt.text,
  images: draft.images.length ? draft.images : prompt.images,
}));

// attachImages adds image files to a draft, refusing other types and images
// beyond the size limit.
export async function attachImages(store: Store, key: string, files: File[]): Promise<void> {
  if (!files.length) return;
  const images = files.filter(file => imageTypes.includes(file.type));
  if (images.length < files.length) showError(store, 'Attach PNG, JPEG, GIF, or WebP images.');
  const unsent = unsentOf(store.get(), key);
  store.update(state => withDraft(state, key, draft => ({...draft, reading: draft.reading + 1})));
  let added: Image[] = [];
  try {
    added = await Promise.all(images.map(readImage));
  } catch (error) {
    showError(store, 'Reading the image failed: ' + (error instanceof Error ? error.message : String(error)));
  }
  // A created or removed new session no longer owns its pending image reads.
  if (isUnsent(key) && unsentOf(store.get(), key)?.identity !== unsent?.identity) return;
  const fits = imageBytes([...draftOf(store.get(), key).images, ...added]) <= maxImageBytes;
  if (!fits) showError(store, 'Images can total at most 10 MiB.');
  store.update(state => withDraft(state, key, draft => ({
    ...draft,
    reading: draft.reading - 1,
    images: fits ? [...draft.images, ...added] : draft.images,
  })));
}

// discardUnsent removes an unsent session and its draft. Its pending image
// reads will be ignored.
export function discardUnsent(state: State, key: string): State {
  const unsentDrafts = {...state.unsentDrafts};
  delete unsentDrafts[key];
  const unsent = state.unsent && unsentKey(state.unsent.workspace) === key ? null : state.unsent;
  return cleared({...state, unsent, unsentDrafts}, key);
}

// unsentCreated gives the created session whatever the user typed in
// the unsent session while its first prompt was sent.
export function unsentCreated(state: State, session: string, key: string): State {
  const {text, images} = draftOf(state, key);
  return withDraft(discardUnsent(state, key), session, () => ({text, images, reading: 0}));
}
