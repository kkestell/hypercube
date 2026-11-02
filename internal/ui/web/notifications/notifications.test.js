import {afterEach, expect, spyOn, test} from 'bun:test';
import {createStore} from '../store.js';
import {cards, closeCard, dismiss, receivedRequests} from './notifications.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const lead = (id, extra = {}) => ({id, name: 'lead', role: 'lead', ...extra});
const workspaces = [
  {id: 'w1', name: 'alpha', sessions: [
    {id: 'a', title: 'A', unseen: true, reply: 'Done.', replySeq: 7, agents: [lead('a-lead'), {id: 'a-w', name: 'worker', role: 'work'}]},
    {id: 'b', title: 'B', unseen: true, reply: 'Also done.', replySeq: 3, agents: [lead('b-lead')]},
  ]},
  {id: 'w2', name: 'beta', sessions: [{id: 'c', title: 'C', unseen: false, agents: [lead('c-lead')]}]},
];
const requests = [
  {agent: 'c-lead', item: {id: 'q2', kind: 'attention', text: 'Allow?', at: 2}},
  {agent: 'a-w', item: {id: 'q1', kind: 'attention', text: 'Proceed?', at: 1}},
];
const state = (extra = {}) => receivedRequests({workspaces, requests: [], selected: '', dismissed: {}, errors: [], ...extra}, requests);

test('request cards come in the server order, then reply cards in sidebar order', () => {
  expect(cards(state()).map(c => c.key)).toEqual(['request:c-lead:q2', 'request:a-w:q1', 'reply:a:7', 'reply:b:3']);
  const [first, second, reply] = cards(state());
  expect(first).toMatchObject({kind: 'request', workspace: 'beta', worker: '', item: requests[0].item});
  expect(first.session.title).toBe('C');
  expect(second.worker).toBe('worker');
  expect(reply).toMatchObject({kind: 'reply', workspace: 'alpha', text: 'Done.'});
});

test('the selected session has no cards', () => {
  expect(cards(state({selected: 'a'})).map(c => c.key)).toEqual(['request:c-lead:q2', 'reply:b:3']);
});

test('dismissals are forgotten once the request is answered or the reply seen', () => {
  let next = ['request:c-lead:q2', 'reply:a:7'].reduce(dismiss, state());
  expect(cards(next).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:b:3']);
  next = receivedRequests(next, requests);
  expect(Object.keys(next.dismissed)).toEqual(['request:c-lead:q2', 'reply:a:7']);
  const seen = workspaces.map(w => ({...w, sessions: w.sessions.map(s => ({...s, unseen: false, replySeq: undefined}))}));
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
  expect(cards(s.get()).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:b:3']);
});
