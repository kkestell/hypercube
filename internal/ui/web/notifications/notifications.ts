// Notification cards, which stack in the top right for the page's errors and
// for the open attention requests and unseen replies of sessions the page is
// not showing. A request comes from the state message's requests, newest
// first; a reply from a session's reply and replySeq. Closing an error card
// removes the error; closing another card dismisses it in this page.
import {chooseSession} from '../sessions/session-actions.ts';
import {allSessions, sessionOf} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import {closeError} from './errors.ts';
import type {AttentionRequest, Card} from './types.ts';

const requestKey = (request: AttentionRequest) => `request:${request.session}:${request.item.id}`;
const replyKey = (session: Session) => `reply:${session.id}:${session.replySeq}`;

// receivedRequests applies a state message's requests. A request's dismissal
// is forgotten once it is answered, and a reply's once its session is seen,
// so the same reply does not return after the agent's busy spells.
export function receivedRequests(state: State, requests: AttentionRequest[]): State {
  const open = new Set(requests.map(requestKey));
  const unseen = new Set(allSessions(state).filter(s => s.unseen).map(s => s.id));
  const kept = (key: string) => key.startsWith('request:') ? open.has(key) : unseen.has(key.split(':')[1]);
  const dismissed: Record<string, true> = {};
  for (const key of Object.keys(state.dismissed)) if (kept(key)) dismissed[key] = true;
  return {...state, requests, dismissed};
}

// cards are the error cards, then the request cards in the server's order,
// then the reply cards in sidebar order.
export function cards(state: State): Card[] {
  const errors = state.errors.map((error): Card => ({key: error.key, kind: 'error', text: error.message}));
  const requests = state.requests.flatMap((request): Card[] => {
    const found = sessionOf(state, request.session);
    if (!found) return [];
    return [{key: requestKey(request), kind: 'request', workspace: found.workspace.name, session: found.session, parent: found.parent, item: request.item}];
  });
  const replies = state.workspaces.flatMap(workspace => workspace.sessions.flatMap(top =>
    [{session: top, parent: null}, ...top.children.map(session => ({session, parent: top}))].filter(({session}) => session.replySeq).map(({session, parent}): Card =>
      ({key: replyKey(session), kind: 'reply', workspace: workspace.name, session, parent, text: session.reply ?? ''}))));
  const shown = [...requests, ...replies].filter(card => card.kind !== 'error' && card.session.id !== state.selected && !state.dismissed[card.key]);
  return [...errors, ...shown];
}

export const dismiss = (state: State, key: string): State => ({...state, dismissed: {...state.dismissed, [key]: true}});

// closeCard removes an error card and dismisses another.
export function closeCard(store: Store, card: Card) {
  if (card.kind === 'error') {
    store.update(state => closeError(state, card.key));
    return;
  }
  store.update(state => dismiss(state, card.key));
}

// startNativeNotifications shows the session of a macOS notification the user
// clicks. The window reports the click in a hypercube-notification event,
// {session}.
export function startNativeNotifications(store: Store) {
  window.addEventListener('hypercube-notification', e => chooseSession(store, e.detail.session));
}
