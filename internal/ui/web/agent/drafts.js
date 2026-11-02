// What the user has typed and attached but not yet sent, by draft key: an
// agent ID for its composer, unsentKey for the unsent session's lead, or
// '<agent>/<request>' for a typed answer to an attention request. Drafts
// outlive their views, so text survives session switches and closed tabs.
import {post} from '../api.js';
import {imageBytes, imageTypes, maxImageBytes, readImage} from '../images.js';
import {showError} from '../notifications/errors.js';
import {createSession} from '../sessions/session-actions.js';
import {unsentKey} from '../sessions/selection.js';

const emptyDraft = {text: '', images: [], reading: 0};

export const draftOf = (state, key) => state.drafts[key] ?? emptyDraft;

// A prompt has text, images, or both, and waits for images still being read.
const hasContent = draft => !draft.reading && (!!draft.text.trim() || draft.images.length > 0);

export const canSend = (state, agent) =>
  hasContent(draftOf(state, agent.id)) && agent.activity !== 'exited' && !(agent.id === unsentKey && state.unsent?.sending);

function withDraft(state, key, change) {
  const draft = change(draftOf(state, key));
  const drafts = {...state.drafts};
  if (draft.text || draft.images.length || draft.reading) drafts[key] = draft;
  else delete drafts[key];
  return {...state, drafts};
}

export const typed = (state, key, text) => withDraft(state, key, draft => ({...draft, text}));

export const removeImage = (state, key, image) => withDraft(state, key, draft => ({...draft, images: draft.images.filter(i => i !== image)}));

// restored puts a refused prompt back, unless the user has started another.
const restored = (state, key, prompt) => withDraft(state, key, draft => ({
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

// sendPrompt sends an agent's draft. The unsent session's first prompt
// creates the session.
export async function sendPrompt(store, agent) {
  const state = store.get();
  if (!canSend(state, agent)) return;
  const {text, images} = draftOf(state, agent.id);
  store.update(next => withDraft(next, agent.id, () => emptyDraft));
  const accepted = agent.id === unsentKey
    ? await createSession(store, {text, images})
    : await post(store, 'agent', {action: 'input', id: agent.id}, JSON.stringify({text, images})) !== null;
  if (!accepted) store.update(next => restored(next, agent.id, {text, images}));
}
