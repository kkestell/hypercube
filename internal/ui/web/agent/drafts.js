// What the user has typed and attached but not yet sent, by draft key: an
// session ID for its composer, unsentKey for the unsent session, or
// '<session>/<request>' for a typed answer to an attention request. Drafts
// outlive their views, so text survives session switches and closed tabs.
import {imageBytes, imageTypes, maxImageBytes, readImage} from '../images.js';
import {showError} from '../notifications/errors.js';

// The unsent session is drawn under this ID and keeps its draft here.
export const unsentKey = 'new';

const emptyDraft = {text: '', images: [], reading: 0};

export const draftOf = (state, key) => state.drafts[key] ?? emptyDraft;

// A prompt has text, images, or both, and waits for images still being read.
const hasContent = draft => !draft.reading && (!!draft.text.trim() || draft.images.length > 0);

export const canSend = (state, session) =>
  !session.ended && hasContent(draftOf(state, session.id)) && session.activity !== 'exited' && !(session.id === unsentKey && state.unsent?.sending);

function withDraft(state, key, change) {
  const draft = change(draftOf(state, key));
  const drafts = {...state.drafts};
  if (draft.text || draft.images.length || draft.reading) drafts[key] = draft;
  else delete drafts[key];
  return {...state, drafts};
}

// cleared empties a draft, as after its prompt is sent.
export const cleared = (state, key) => withDraft(state, key, () => emptyDraft);

export const typed = (state, key, text) => withDraft(state, key, draft => ({...draft, text}));

export const removeImage = (state, key, image) => withDraft(state, key, draft => ({...draft, images: draft.images.filter(i => i !== image)}));

// restored puts a refused prompt back, unless the user has started another.
export const restored = (state, key, prompt) => withDraft(state, key, draft => ({
  ...draft,
  text: draft.text || prompt.text,
  images: draft.images.length ? draft.images : prompt.images,
}));

// attachImages adds image files to a draft, refusing other types and images
// beyond the size limit.
export async function attachImages(store, key, files) {
  if (!files.length) return;
  const images = files.filter(file => imageTypes.includes(file.type));
  if (images.length < files.length) showError(store, 'Attach PNG, JPEG, GIF, or WebP images.');
  const unsent = store.get().unsent;
  store.update(state => withDraft(state, key, draft => ({...draft, reading: draft.reading + 1})));
  let added = [];
  try {
    added = await Promise.all(images.map(readImage));
  } catch (error) {
    showError(store, 'Reading the image failed: ' + error.message);
  }
  // Closing the unsent session discards its draft.
  if (key === unsentKey && store.get().unsent?.identity !== unsent.identity) return;
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
export function unsentCreated(state, session) {
  const {text, images} = draftOf(state, unsentKey);
  return withDraft(cleared(state, unsentKey), session, () => ({text, images, reading: 0}));
}
