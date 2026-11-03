import {afterEach, expect, spyOn, test} from 'bun:test';
import {createStore} from '../store.js';
import {cards, closeCard, dismiss, receivedRequests, startNativeNotifications} from './notifications.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const workspaces = [
  {id: 'w1', name: 'alpha', sessions: [
    {id: 'a', title: 'A', unseen: true, reply: 'Done.', replySeq: 7, children: [{id: 'a-w', parent: 'a', title: 'worker', unseen: true, reply: 'Worked.', replySeq: 4, children: []}]},
    {id: 'b', title: 'B', unseen: true, reply: 'Also done.', replySeq: 3, children: []},
  ]},
  {id: 'w2', name: 'beta', sessions: [{id: 'c', title: 'C', unseen: false, children: []}]},
];
const requests = [
  {session: 'c', item: {id: 'q2', kind: 'attention', text: 'Allow?', at: 2}},
  {session: 'a-w', item: {id: 'q1', kind: 'attention', text: 'Proceed?', at: 1}},
];
const state = (extra = {}) => receivedRequests({workspaces, requests: [], selected: '', dismissed: {}, errors: [], ...extra}, requests);

test('request cards come in the server order, then reply cards in sidebar order', () => {
  expect(cards(state()).map(c => c.key)).toEqual(['request:c:q2', 'request:a-w:q1', 'reply:a:7', 'reply:a-w:4', 'reply:b:3']);
  const [first, second, reply, nested] = cards(state());
  expect(first).toMatchObject({kind: 'request', workspace: 'beta', item: requests[0].item});
  expect(first.session.title).toBe('C');
  expect(second.session.title).toBe('worker');
  expect(second.parent.title).toBe('A');
  expect(reply).toMatchObject({kind: 'reply', workspace: 'alpha', text: 'Done.'});
  expect(nested).toMatchObject({kind: 'reply', workspace: 'alpha', text: 'Worked.'});
  expect(nested.parent.title).toBe('A');
});

test('the selected session has no cards; its nested sessions keep theirs', () => {
  expect(cards(state({selected: 'a'})).map(c => c.key)).toEqual(['request:c:q2', 'request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
  expect(cards(state({selected: 'a-w'})).map(c => c.key)).toEqual(['request:c:q2', 'reply:a:7', 'reply:b:3']);
});

test('dismissals are forgotten once the request is answered or the reply seen', () => {
  let next = ['request:c:q2', 'reply:a:7'].reduce(dismiss, state());
  expect(cards(next).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
  next = receivedRequests(next, requests);
  expect(Object.keys(next.dismissed)).toEqual(['request:c:q2', 'reply:a:7']);
  const unread = s => ({...s, unseen: false, replySeq: undefined});
  const seen = workspaces.map(w => ({...w, sessions: w.sessions.map(s => ({...unread(s), children: s.children.map(unread)}))}));
  next = receivedRequests({...next, workspaces: seen}, requests.slice(1));
  expect(next.dismissed).toEqual({});
});

test('closing a reply card marks it seen; closing a request card does not', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const s = createStore(state());
  const [request, , reply] = cards(s.get());
  closeCard(s, request);
  expect(fetchMock).not.toHaveBeenCalled();
  closeCard(s, reply);
  const url = new URL(fetchMock.mock.calls[0][0], 'http://x/');
  expect([url.pathname, url.searchParams.get('action'), url.searchParams.get('id')]).toEqual(['/session', 'seen', 'a']);
  expect(cards(s.get()).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
});

test('a clicked macOS notification shows its session', () => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const s = createStore(state({selected: 'c'}));
  startNativeNotifications(s);
  dispatchEvent(new CustomEvent('hypercube-notification', {detail: {session: 'b'}}));
  expect(s.get().selected).toBe('b');
});
