// Notification cards, which stack in the top right for the open attention
// requests and unseen lead replies of sessions the page is not showing. A
// request comes from the state message's requests, newest first; a reply from
// a session's reply and replySeq. Closing a card dismisses it in this page.
import {markSeen} from '../sessions/session-actions.js';
import {allSessions, sessionOfAgent} from '../sessions/selection.js';

const requestKey = request => `request:${request.agent}:${request.item.id}`;
const replyKey = session => `reply:${session.id}:${session.replySeq}`;

// receivedRequests applies a state message's requests. A request's dismissal
// is forgotten once it is answered, and a reply's once its session is seen,
// so the same reply does not return after its lead's busy spells.
export function receivedRequests(state, requests) {
  const open = new Set(requests.map(requestKey));
  const unseen = new Set(allSessions(state).filter(s => s.unseen).map(s => s.id));
  const kept = key => key.startsWith('request:') ? open.has(key) : unseen.has(key.split(':')[1]);
  const dismissed = Object.fromEntries(Object.keys(state.dismissed).filter(kept).map(key => [key, true]));
  return {...state, requests, dismissed};
}

// cards are the request cards in the server's order, then the reply cards in
// sidebar order.
export function cards(state) {
  const requests = state.requests.flatMap(request => {
    const found = sessionOfAgent(state, request.agent);
    if (!found) return [];
    const worker = found.agent.role === 'lead' ? '' : found.agent.name;
    return [{key: requestKey(request), kind: 'request', workspace: found.workspace.name, session: found.session, worker, agent: request.agent, item: request.item}];
  });
  const replies = state.workspaces.flatMap(workspace => workspace.sessions.filter(s => s.replySeq).map(session =>
    ({key: replyKey(session), kind: 'reply', workspace: workspace.name, session, text: session.reply})));
  return [...requests, ...replies].filter(card => card.session.id !== state.selected && !state.dismissed[card.key]);
}

export const dismiss = (state, key) => ({...state, dismissed: {...state.dismissed, [key]: true}});

// closeCard dismisses a card. Closing a reply's card also marks it seen, which
// lasts across reloads.
export function closeCard(store, card) {
  store.update(state => dismiss(state, card.key));
  if (card.kind === 'reply') markSeen(card.session.id);
}
