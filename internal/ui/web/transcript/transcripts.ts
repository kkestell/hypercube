// The attached session's transcript as {order, items}: one
// item per ID, ordered by seq. The server sends the session's items whole after
// the page selects a session, then each new or changed item.
import type {TranscriptMessage} from '../data-socket.ts';
import type {State} from '../state.ts';
import {imageSrc} from '../images.ts';
import {reconciledSubmissions, submissionsOf} from '../sessions/submissions.ts';
import type {Transcript} from './types.ts';

export const emptyTranscript: Transcript = {order: [], items: {}};

// The attached transcript stays authoritative. Page-owned submissions append
// only until the corresponding user item arrives, including on a reset.
export function displayedTranscript(state: State, key: string): Transcript {
  const transcript = state.session?.id === key ? state.session.transcript : emptyTranscript;
  const provisional = submissionsOf(state, key).filter(entry => !transcript.items[entry.id]);
  if (!provisional.length) return transcript;
  const items = {...transcript.items};
  let seq = transcript.order.length ? items[transcript.order.at(-1)!].seq : 0;
  for (const entry of provisional) items[entry.id] = {id: entry.id, kind: 'user', text: entry.text, images: entry.images.map(imageSrc).filter(src => src !== null), done: true, seq: ++seq};
  return {order: [...transcript.order, ...provisional.map(entry => entry.id)], items};
}

// receivedTranscript applies a transcript message for the attached session.
export function receivedTranscript(state: State, message: TranscriptMessage): State {
  const session = state.session;
  if (session?.id !== message.session) return state;
  const old = message.reset ? emptyTranscript : session.transcript;
  const items = {...old.items};
  for (const item of message.items) items[item.id] = item;
  const order = Object.keys(items).sort((a, b) => items[a].seq - items[b].seq);
  const transcript = {order, items};
  return {...reconciledSubmissions(state, message.session, order), session: {...session, transcript, transcriptReady: true}};
}

// setExpanded opens or closes an activity or a message.
export function setExpanded(state: State, key: string, open: boolean): State {
  const session = state.session;
  if (!session || !!session.expanded[key] === open) return state;
  const expanded = {...session.expanded};
  if (open) expanded[key] = true;
  else delete expanded[key];
  return {...state, session: {...session, expanded}};
}
