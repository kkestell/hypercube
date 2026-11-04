// What the user has typed and attached but not yet sent, by draft key: an
// session ID for its composer, unsentKey for the unsent session, or
// '<session>/<request>' for a typed answer to an attention request. Drafts
// outlive their views, so text survives session switches and closed tabs.
import {imageBytes, type Image, imageTypes, maxImageBytes, readImage} from '../images.ts';
import {showError} from '../notifications/errors.ts';
import type {Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {Draft} from './types.ts';

// The unsent session is drawn under this ID and keeps its draft here.
export const unsentKey = 'new';

const emptyDraft: Draft = {text: '', images: [], reading: 0};

export const draftOf = (state: State, key: string): Draft => state.drafts[key] ?? emptyDraft;

// A prompt has text, images, or both, and waits for images still being read.
const hasContent = (draft: Draft) => !draft.reading && (!!draft.text.trim() || draft.images.length > 0);

// A new session needs a workspace and profile before its first prompt can send.
const unsentReady = (state: State) => !!state.unsent?.workspace && !!state.unsent.profile && !state.unsent.sending;

export const canSend = (state: State, session: Pick<Session, 'id' | 'ended' | 'activity'>, text = draftOf(state, session.id).text) =>
  !session.ended && hasContent({...draftOf(state, session.id), text}) && session.activity !== 'exited' && (session.id !== unsentKey || unsentReady(state));

function withDraft(state: State, key: string, change: (draft: Draft) => Draft): State {
  const draft = change(draftOf(state, key));
  const drafts = {...state.drafts};
  if (draft.text || draft.images.length || draft.reading) drafts[key] = draft;
  else delete drafts[key];
  return {...state, drafts};
}

// cleared empties a draft, as after its prompt is sent.
export const cleared = (state: State, key: string): State => withDraft(state, key, () => emptyDraft);

export const typed = (state: State, key: string, text: string): State => withDraft(state, key, draft => ({...draft, text}));

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
  const unsent = store.get().unsent;
  store.update(state => withDraft(state, key, draft => ({...draft, reading: draft.reading + 1})));
  let added: Image[] = [];
  try {
    added = await Promise.all(images.map(readImage));
  } catch (error) {
    showError(store, 'Reading the image failed: ' + (error instanceof Error ? error.message : String(error)));
  }
  // Closing the unsent session discards its draft.
  if (key === unsentKey && store.get().unsent?.identity !== unsent?.identity) return;
  const fits = imageBytes([...draftOf(store.get(), key).images, ...added]) <= maxImageBytes;
  if (!fits) showError(store, 'Images can total at most 10 MiB.');
  store.update(state => withDraft(state, key, draft => ({
    ...draft,
    reading: draft.reading - 1,
    images: fits ? [...draft.images, ...added] : draft.images,
  })));
}

// unsentCreated gives the created session whatever the user typed in
// the unsent session while its first prompt was sent. Pending image reads
// belong to the discarded unsent session and will be ignored.
export function unsentCreated(state: State, session: string): State {
  const {text, images} = draftOf(state, unsentKey);
  return withDraft(cleared(state, unsentKey), session, () => ({text, images, reading: 0}));
}
