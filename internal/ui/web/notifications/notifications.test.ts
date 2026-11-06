import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import type {Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {cards, closeCard, dismiss, receivedRequests, startNativeNotifications} from './notifications.ts';
import type {AttentionRequest} from './types.ts';

afterEach(() => mock.restore());

const workspaces = [
  testWorkspace({id: 'w1', name: 'alpha', sessions: [
    testSession({id: 'a', title: 'A', unseen: true, reply: 'Done.', replySeq: 7, children: [
      testSession({id: 'a-w', parent: 'a', title: 'worker', unseen: true, reply: 'Worked.', replySeq: 4}),
    ]}),
    testSession({id: 'b', title: 'B', unseen: true, reply: 'Also done.', replySeq: 3}),
  ]}),
  testWorkspace({id: 'w2', name: 'beta', sessions: [testSession({id: 'c', title: 'C'})]}),
];
const requests: AttentionRequest[] = [
  {session: 'c', item: {id: 'q2', kind: 'attention', text: 'Allow?', at: 2}},
  {session: 'a-w', item: {id: 'q1', kind: 'attention', text: 'Proceed?', at: 1}},
];
const state = (extra: Partial<State> = {}) => receivedRequests(testState({workspaces, ...extra}), requests);

test('request cards come in the server order, then reply cards in sidebar order', () => {
  expect(cards(state()).map(c => c.key)).toEqual(['request:c:q2', 'request:a-w:q1', 'reply:a:7', 'reply:a-w:4', 'reply:b:3']);
  const [first, second, reply, nested] = cards(state());
  expect(first).toMatchObject({kind: 'request', workspace: 'beta', item: requests[0].item, session: {title: 'C'}});
  expect(second).toMatchObject({session: {title: 'worker'}, parent: {title: 'A'}});
  expect(reply).toMatchObject({kind: 'reply', workspace: 'alpha', text: 'Done.'});
  expect(nested).toMatchObject({kind: 'reply', workspace: 'alpha', text: 'Worked.', parent: {title: 'A'}});
});

test('the selected session has no cards; its nested sessions keep theirs', () => {
  expect(cards(state({selected: 'a'})).map(c => c.key)).toEqual(['request:c:q2', 'request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
  expect(cards(state({selected: 'a-w'})).map(c => c.key)).toEqual(['request:c:q2', 'reply:a:7', 'reply:b:3']);
});

test('the notification settings turn request and reply cards off', () => {
  const off = (requests: boolean, replies: boolean) =>
    cards(state({config: testConfig({notifications: {requests, replies, macos: true, sound: true}})})).map(c => c.key);
  expect(off(false, true)).toEqual(['reply:a:7', 'reply:a-w:4', 'reply:b:3']);
  expect(off(true, false)).toEqual(['request:c:q2', 'request:a-w:q1']);
});

test('dismissals are forgotten once the request is answered or the reply seen', () => {
  let next = ['request:c:q2', 'reply:a:7'].reduce(dismiss, state());
  expect(cards(next).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
  next = receivedRequests(next, requests);
  expect(Object.keys(next.dismissed)).toEqual(['request:c:q2', 'reply:a:7']);
  const unread = (s: Session): Session => ({...s, unseen: false, replySeq: undefined});
  const seen = workspaces.map(w => ({...w, sessions: w.sessions.map(s => ({...unread(s), children: s.children.map(unread)}))}));
  next = receivedRequests({...next, workspaces: seen}, requests.slice(1));
  expect(next.dismissed).toEqual({});
});

test('closing reply and request cards dismisses them without marking replies seen', () => {
  const fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const s = createStore(state());
  const [request, , reply] = cards(s.get());
  closeCard(s, request);
  expect(fetchMock).not.toHaveBeenCalled();
  closeCard(s, reply);
  expect(fetchMock).not.toHaveBeenCalled();
  expect(s.get().workspaces[0].sessions[0].unseen).toBe(true);
  expect(cards(s.get()).map(c => c.key)).toEqual(['request:a-w:q1', 'reply:a-w:4', 'reply:b:3']);
});

test('a clicked macOS notification shows its session', () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const s = createStore(state({selected: 'c'}));
  startNativeNotifications(s);
  dispatchEvent(new CustomEvent('hypercube-notification', {detail: {session: 'b'}}));
  expect(s.get().selected).toBe('b');
});
