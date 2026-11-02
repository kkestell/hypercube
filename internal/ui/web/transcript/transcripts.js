// The attached session's transcripts, one per agent as {order, items}: one
// item per ID, ordered by seq. The server sends each agent's items whole after
// the page selects a session, then each new or changed item.
import {ancestorKeys} from './transcript-tree.js';

const empty = {order: [], items: {}};

// receivedTranscript applies a transcript message for the attached session.
// An open attention request opens the runs and tool calls around it.
export function receivedTranscript(state, message) {
  const session = state.session;
  if (session?.id !== message.session) return state;
  const old = message.reset ? empty : session.transcripts[message.agent] ?? empty;
  const items = {...old.items};
  for (const item of message.items) items[item.id] = item;
  const order = Object.keys(items).sort((a, b) => items[a].seq - items[b].seq);
  const transcript = {order, items};
  const expanded = {...session.expanded};
  for (const item of Object.values(items)) {
    if (item.kind !== 'attention' || item.done) continue;
    for (const key of ancestorKeys(transcript, item.id)) expanded[key] = true;
  }
  return {...state, session: {...session, transcripts: {...session.transcripts, [message.agent]: transcript}, expanded}};
}

// setExpanded opens or closes a tool call, run, message, or Thinking item.
export function setExpanded(state, key, open) {
  const session = state.session;
  if (!session || !!session.expanded[key] === open) return state;
  const expanded = {...session.expanded};
  if (open) expanded[key] = true;
  else delete expanded[key];
  return {...state, session: {...session, expanded}};
}
